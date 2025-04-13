package main

import (
	"context"
	_ "database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"         // Or your chosen driver
	"github.com/jackc/pgx/v5/pgxpool" // Or your chosen driver

	//"github.com/jackc/pgx/v5/pgtype" // Or your chosen driver
	"github.com/labstack/echo/v4"

	//"github.com/labstack/echo/v4/middleware"
	"os"
	"time"
)

/*
	{
	   ... other json fields
	  "geometry": {
	    "type": "Polygon",
	    "coordinates": [[
	      [-74.5, 40.5],   // First point (longitude, latitude)
	      [-74.3, 40.5],   // Second point
	      [-74.3, 40.7],   // Third point
	      [-74.5, 40.7],   // Fourth point
	      [-74.5, 40.5]    // Back to first point (closing the polygon)
	    ]]
	  }
	}
*/
type Geometry struct {
	Type        string        `json:"type"`
	Coordinates [][][]float64 `json:"coordinates"`
}

type SearchDoc struct {
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date"`
	Geometry    *Geometry `json:"geometry"`
	CameraNames []string  `json:"camera_names"`
	PlateCode   string    `json:"plate_code"`
	Make        string    `json:"make"`
	VehicleType string    `json:"vehicle_type"`
	Color       string    `json:"color"`
	PlateNum    string    `json:"plate_num"`
	Page        int       `json:"page"`
	PageSize    int       `json:"page_size"`
}

/*
{
  "metadata": {
    "page_count": 50    // Total pages on first request, -1 for subsequent pages
  },
  "results": [
    {
      "read_time": "2024-02-01T10:30:45Z",
      "camera_name": "Mt Pleasant (Eastbound)",
      "plate_num": "ABC123",
      "plate_code": "USA-NY",
      "make": "Toyota",
      "vehicle_type": "Sedan",
      "color": "Blue",
      "location": {
        "lat": 40.7128,
        "lon": -74.0060
      },
      "plate_img": "https://bucket-name.s3.wasabisys.com/plates/123.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&[...]",
      "full_img": "https://bucket-name.s3.wasabisys.com/vehicles/123.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&[...]"
    }
  ]
}
*/

type SearchResults struct {
	Metadata    Metadata     `json:"metadata"`
	AlprRecords []AlprRecord `json:"results"`
}
type Metadata struct {
	PageCount int `json:"page_count"`
}

// NOTE: using pointers so that any db null values will be set to null as the json value. The default serialization for SqlNullString is trash.
type AlprRecord struct {
	PlateNum    *string         `db:"plate_num"     json:"plate_num"`
	PlateCode   *string         `db:"plate_code"    json:"plate_code"`
	CameraName  *string         `db:"camera_name"   json:"camera_name"`
	ReadID      *string         `db:"read_id"       json:"read_id"`
	ReadTime    time.Time       `db:"read_time"     json:"read_time"`
	ImageID     *string         `db:"image_id"      json:"image_id"`
	Location    json.RawMessage `db:"location"      json:"location"`
	Make        *string         `db:"make"          json:"make"`
	VehicleType *string         `db:"vehicle_type"  json:"vehicle_type"`
	Color       *string         `db:"color"         json:"color"`
	SourceID    *string         `db:"source_id"     json:"source_id"`
}

//	"error": {
//	    "code": "INVALID_SEARCH",
//	    "message": "Invalid date range specified",
//	    "details": "End date must be after start date"
//	  }
type ErrorRes struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

type App struct {
	DB   *pgxpool.Pool
	Echo *echo.Echo
}

func initApp() *App {
	//pass query to db to get stuff
	connStr := os.Getenv("ALPR_DB")

	if connStr == "" {
		connStr = "postgresql://admin:admin@192.168.3.225:5533/snap"
	}

	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %w", err)
	}

	//check that db was connected
	if err := dbPool.Ping(ctx); err != nil {
		log.Fatalf("unable to ping database: %v", err)
	}

	e := echo.New()

	// Middleware
	// e.Use(middleware.Logger())
	// e.Use(middleware.Recover())
	// e.Use(middleware.CORS())

	app := &App{
		DB:   dbPool,
		Echo: e,
	}

	registerRoutes(app)
	return app

}

func registerRoutes(app *App) {
	app.Echo.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	api := app.Echo.Group("/api")
	api.POST("/alpr/v1/search", app.search)
}

func (app *App) search(c echo.Context) error {
	ctx := c.Request().Context()

	//TODO: consider attaching a prepared search query to the App struct. perf optimization. not yet since it might not be needed.
	//get the json from the RequestBody and convert to SearchDocument struct

	searchDoc := SearchDoc{}
	if err := c.Bind(&searchDoc); err != nil {
		//return c.String(http.StatusBadRequest, "Bad request" )
		errMsg := ErrorRes{
			Code:    "BAD_REQUEST",
			Message: "Bad Search Document",
			Details: "Couldn't parse SearchDoc. Check that you've included all required fields",
		}
		//return c.String(http.StatusBadRequest, "Bad request" )
		return c.JSON(http.StatusBadRequest, errMsg)
	}

	//build the query
	query, q_vals, err := BuildSelectQuery(searchDoc)
	if err != nil {
		errMsg := ErrorRes{
			Code:    "BAD_REQUEST",
			Message: "Bad Search Document",
			Details: "Couldn't convert SearchDoc to query. Check that you've included all required fields",
		}
		return c.JSON(http.StatusBadRequest, errMsg)
	}

	rows, err := app.DB.Query(ctx, query, q_vals...)
	if err != nil {
		errMsg := ErrorRes{
			Code:    "INTERNAL_SERVER_ERROR",
			Message: "Failed to execute query",
			Details: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, errMsg)
	}
	defer rows.Close()

	//helper to scan rows into struct directly
	alprRecords, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[AlprRecord])
	if err != nil {
		// Check specifically for no rows found, which might not be an "error"
		if err == pgx.ErrNoRows {
			//TODO: return empty json results?
			fmt.Println("No search results found .")
		} else {
			fmt.Fprintf(os.Stderr, "Failed to collect rows: %v\n", err)

			errMsg := ErrorRes{
				Code:    "INTERNAL_SERVER_ERROR",
				Message: "Failed to collect query results",
				Details: err.Error(),
				//helper to scan rows into struct directly
				//helper to scan rows into struct directly
			}
			return c.JSON(http.StatusInternalServerError, errMsg)
		}
	}

	fmt.Println("recoreds retrieved")

	//TODO: postprocessing

	//add custom fields, generate presigned url.
	return c.JSON(200, alprRecords)
}

func main() {

	app := initApp()
	defer app.DB.Close()

	port := os.Getenv("ALPR_PORT")
	if port == "" {
		port = "8080"
	}

	log.Fatal(app.Echo.Start(":" + port))

}
