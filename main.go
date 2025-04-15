package main

import (
	"context"
	_ "database/sql"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"log"
	"net/http"
	"os"
	"time"
)

//Example of what Geometry json looks like
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
	//for limit/offset paging
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	//for cursor based paging (not implemented yet)
	Direction     string `json:"direction,omitempty"`
	NextPageToken string `json:"next_page_token,omitempty"`
	PrevPageToken string `json:"prev_page_token,omitempty"`
}

/* Example of what SearchResults json looks like
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
	PageCount int64 `json:"page_count"`
}

// NOTE: using pointers so that any db null values will be set to null as the json value. The default serialization for SqlNullString is trash.
type AlprRecord struct {
	PlateNum    *string         `db:"plate_num"     json:"plate_num"`
	PlateCode   *string         `db:"plate_code"    json:"plate_code"`
	CameraName  *string         `db:"camera_name"   json:"camera_name"`
	ReadID      *string         `db:"read_id"       json:"read_id"`
	ReadTime    time.Time       `db:"read_time"     json:"read_time"`
	ImageID     *string         `db:"image_id"      json:"image_id"`
	Location    json.RawMessage `db:"location"      json:"location"` //a passthrough value. location is returned as a block of json.
	Make        *string         `db:"make"          json:"make"`
	VehicleType *string         `db:"vehicle_type"  json:"vehicle_type"`
	Color       *string         `db:"color"         json:"color"`
	SourceID    *string         `db:"source_id"     json:"source_id"`
	PlateImg    string          `json:"plate_img"`
	FullImg     string          `json:"full_img"`
	SiteID      string          `json:"site_id"`
	UserID      *string         `json:"user_id"`
	AgencyName  string          `json:"agency_name"`
}

type ErrorRes struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

type App struct {
	DB     *pgxpool.Pool
	Echo   *echo.Echo
	Wasabi *Wasabi
}

func initApp() *App {

	connStr := getEnv("ALPR_DB", "postgresql://admin:admin@192.168.3.225:5533/snap")

	log.Println("------------- starting application ------------")
	log.Printf("conn str: %s", connStr)
	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %w", err)
	}

	//check that db was connected
	if err := dbPool.Ping(ctx); err != nil {
		log.Fatalf("unable to ping database: %v", err)
	}

	//where our images live.
	s3_host := getEnv("S3_HOST", "s3.wasabisys.com")
	s3_region := getEnv("S3_REGION", "us-east-1")

	wasabi, err := NewWasabi(s3_host, s3_region)
	if err != nil {
		log.Fatalf("unable to create wasabi client: %v", err)
	}

	e := echo.New()

	// Middleware
	// e.Use(middleware.Logger())
	// e.Use(middleware.Recover())
	// e.Use(middleware.CORS())

	app := &App{
		DB:     dbPool,
		Echo:   e,
		Wasabi: wasabi,
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

/*
	search: the main event!

- recieve a json request body representing an alpr search (plate num partial matches, date ranges, geo searches, vehicle characteristics, etc),
- convert json to a SearchDoc struct and build a postgres query from it,
- get the query results and build a SearchResults struct with Metadata (page count) and all the AlprRecords.
- Postprocess AlprRecords by generating presigned_urls for secure access to images on wasabi (s3) without needing to authenticate (build into the presigned link)
- Return SearchResults.
*/
func (app *App) search(c echo.Context) error {
	ctx := c.Request().Context()
	log.Println("Search requested...")
	//TODO: consider attaching a prepared search query to the App struct. perf optimization. not yet since it might not be needed.

	searchDoc := SearchDoc{}
	if err := c.Bind(&searchDoc); err != nil {
		errMsg := ErrorRes{
			Code:    "BAD_REQUEST",
			Message: "Bad Search Document",
			Details: "Couldn't parse SearchDoc. Check that you've included all required fields",
		}
		return c.JSON(http.StatusBadRequest, errMsg)
	}

	if searchDoc.PageSize > 1000 {
		searchDoc.PageSize = 1000
	}

	query, err := BuildSelectQuery(searchDoc)
	if err != nil {
		errMsg := ErrorRes{
			Code:    "BAD_REQUEST",
			Message: "Bad Search Document",
			Details: "Couldn't convert SearchDoc to query. Check that you've included all required fields",
		}
		return c.JSON(http.StatusBadRequest, errMsg)
	}

	rows, err := app.DB.Query(ctx, query.Text, query.Params...)
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

	//post process Add the presigned urls to each record for public image retrieval. add hardcoded required vals
	for i := 0; i < len(alprRecords); i++ {
		//TODO: remember this is temporary hardcoding.
		alprRecords[i].SiteID = "NJ0141000"
		alprRecords[i].AgencyName = "East Hanover"

		//Remember the pain of not deferencing a ptr!
		sourceIDPtr := alprRecords[i].SourceID
		imageIDPtr := alprRecords[i].ImageID
		readIDPtr := alprRecords[i].ReadID

		//will there always be a SourceID and an ImageID? i believe so but check anyway.
		if sourceIDPtr == nil {
			log.Println("skipping presign. no sourceid") //can't do nothing without the sourceid
			continue
		}

		//verify full image
		if imageIDPtr != nil {
			full_img := fmt.Sprintf("alpr/%s/%s", *sourceIDPtr, *imageIDPtr)
			full_url, err := app.Wasabi.PresignUrl("njsnap", full_img)
			alprRecords[i].FullImg = full_url
			if err != nil {
				log.Printf("%v\n", err)
			}
		}

		//verify plate image
		if readIDPtr != nil {
			plate_img := fmt.Sprintf("alpr-plate/%s/%s", *sourceIDPtr, *readIDPtr)
			plate_url, err := app.Wasabi.PresignUrl("njsnap", plate_img)
			if err != nil {
				log.Println("no plate url present")
			}

			alprRecords[i].PlateImg = plate_url
		}
	}

	count := int64(-1)

	//we only get a count for the first page. client holds on to it until a new 1st page is requested.
	//saves us from doing work we don't need to
	if searchDoc.Page == 1 {
		cq, _ := BuildCountQuery(searchDoc)

		row := app.DB.QueryRow(ctx, cq.Text, cq.Params...)
		if err := row.Scan(&count); err != nil {
			count = 0
		}
	}

	total := CalculateTotalPages(count, searchDoc.PageSize)
	fmt.Printf("total items: %d , total pages: %d\n", count, total)

	results := SearchResults{
		Metadata:    Metadata{PageCount: count},
		AlprRecords: alprRecords,
	}

	return c.JSON(200, results)
}

// helper
func getEnv(key string, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func main() {
	app := initApp()
	defer app.DB.Close()
	port := getEnv("ALPR_PORT", "8080")
	log.Fatal(app.Echo.Start(":" + port))
}
