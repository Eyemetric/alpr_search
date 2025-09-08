package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Eyemetric/alpr_service/internal/api/alert"
	"github.com/Eyemetric/alpr_service/internal/api/hotlist"
	"github.com/Eyemetric/alpr_service/internal/api/plates"
	"github.com/Eyemetric/alpr_service/internal/api/search"
	"github.com/Eyemetric/alpr_service/internal/api/wasabi"
	"github.com/Eyemetric/alpr_service/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

type App struct {
	DB     *pgxpool.Pool //TODO: this will be removed when everything is moved to the repo
	Echo   *echo.Echo
	Wasabi *wasabi.Wasabi
	//Repo    *repository.PgxAlprRepo
	Repo    repository.ALPRRepository
	Context context.Context
}

type ErrorRes struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details"`
}

func initApp() *App {

	//config
	connStr := getEnv("ALPR_DB", "postgresql://admin:admin@192.168.3.225:5533/snap")
	s3_host := getEnv("S3_HOST", "s3.wasabisys.com")
	s3_region := getEnv("S3_REGION", "us-east-1")
	plateHitUrl := getEnv("PLATEHIT_URL", "https://demo.njroic.net/api/poi/alpr")
	njsnapToken := getEnv("NJSNAP_TOKEN", "1234")

	log.Println("------------- starting application ------------")
	log.Printf("conn str: %s\n", connStr)
	log.Printf("state url: %s\n", plateHitUrl)
	log.Printf("state token: %s\n", njsnapToken)

	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %e", err)
	}

	//check that db was connected
	if err := dbPool.Ping(ctx); err != nil {
		log.Fatalf("unable to ping database: %v", err)
	}

	wasabi, err := wasabi.NewWasabi(s3_host, s3_region)
	if err != nil {
		log.Fatalf("unable to create wasabi client: %v", err)
	}

	e := echo.New()

	repo := repository.NewPgxAlprRepo(dbPool)

	alertConfig := alert.AlertConfig{
		PlateHitUrl: plateHitUrl,
		AuthToken:   njsnapToken,
		SendTimeout: 60 * time.Second,
	}

	app := &App{
		DB:      dbPool,
		Echo:    e,
		Wasabi:  wasabi,
		Repo:    repo,
		Context: ctx,
	}

	registerRoutes(app)
	startAlertListener(app, alertConfig)
	return app

}

func startAlertListener(app *App, conf alert.AlertConfig) {

	err := alert.StartAlertListener(app.Context, app.Repo, app.Wasabi, conf)
	if err != nil {
		log.Fatalf("could not start alert listener: %v", err)
	}
}

func registerRoutes(app *App) {
	app.Echo.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	http_api := app.Echo.Group("/api")
	http_api.POST("/alpr/v1/search", app.search)
	http_api.POST("/alpr/v1/add", app.addPlate)
	http_api.POST("/alpr/v1/hotlist", app.addHotlist)
}

func (app *App) health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (app *App) addHotlist(c echo.Context) error {

	log.Println("adding to hotlist...")
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return err
	}

	fmt.Println(string(body))

	err = hotlist.AddHotlist(app.Context, body, app.Repo)

	if err != nil {
		errMsg := ErrorRes{
			Code:    "INTERNAL_SERVER_ERROR",
			Message: "Could not add to hotlist",
			Details: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, errMsg)
	}

	return c.JSON(200, "")
}

func (app *App) addPlate(c echo.Context) error {

	log.Println("Adding a plate")

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return nil
	}

	err = plates.AddPlate(app.Context, body, app.Repo)
	if err != nil {
		errMsg := ErrorRes{
			Code:    "INTERNAL_SERVER_ERROR",
			Message: "Could not add plate",
			Details: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, errMsg)
	}

	return c.JSON(200, "")
}

/*
- recieve a json request body representing an alpr search (plate num partial matches, date ranges, geo searches, vehicle characteristics, etc),
- convert json to a SearchDoc struct and build a postgres query from it,
- get the query results and build a SearchResults struct with Metadata (page count) and all the matchin AlprRecords.
- Postprocess AlprRecords:
  - generate presigned_urls for secure access to images on wasabi (s3) without needing to authenticate (build into the presigned link)
  - set static values for site_id, agency_name (temporary)

- Return SearchResults.
*/
func (app *App) search(c echo.Context) error {
	ctx := c.Request().Context()
	log.Println("Search requested...")
	//TODO: consider attaching a prepared search query to the App struct. perf optimization. not yet since it might not be needed.

	searchDoc := search.SearchDoc{}
	//parses json body into a SearchDoc
	if err := c.Bind(&searchDoc); err != nil {
		fmt.Print(err)
		errMsg := ErrorRes{
			Code:    "BAD_REQUEST",
			Message: "Bad Search Document",
			Details: "Couldn't parse SearchDoc. Check that you've included all required fields, and the search value are correct",
		}
		return c.JSON(http.StatusBadRequest, errMsg)
	}

	//limit max page size
	if searchDoc.PageSize > 1000 {
		searchDoc.PageSize = 1000
	}

	//transform the searchDoc into a SQL query
	query, err := search.BuildSelectQuery(searchDoc)
	if err != nil {
		errMsg := ErrorRes{
			Code:    "BAD_REQUEST",
			Message: "Bad Search Document",
			Details: "Couldn't convert SearchDoc to query. Check that you've included all required fields, and the search value are correct",
		}
		return c.JSON(http.StatusBadRequest, errMsg)
	}

	//get the SearchResults
	rows, err := app.DB.Query(ctx, query.Text, query.Params...)
	if err != nil {
		errMsg := ErrorRes{
			Message: "Failed to execute query",
			Details: err.Error(),
		}
		return c.JSON(http.StatusInternalServerError, errMsg)
	}
	defer rows.Close()

	//Transform db rows into an array of AlprRecord structs
	alprRecords, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[search.AlprRecord])
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

	//post process:  Generate presigned urls for each record
	//add hardcoded required vals
	for i := 0; i < len(alprRecords); i++ {
		//TODO: remember! this is temporary hardcoding.
		alprRecords[i].SiteID = "NJ0141000"
		alprRecords[i].AgencyName = "East Hanover Township Police Department"

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
	//saves us from makeing extra queries to calculate total pages.
	if searchDoc.Page == 1 {
		cq, _ := search.BuildCountQuery(searchDoc)

		row := app.DB.QueryRow(ctx, cq.Text, cq.Params...)
		if err := row.Scan(&count); err != nil {
			count = 0
		}
	}

	//returns -1 if given -1
	total := search.CalculateTotalPages(count, searchDoc.PageSize)
	fmt.Printf("total items: %d , total pages: %d\n", count, total)

	results := search.SearchResults{
		Metadata:    search.Metadata{PageCount: count},
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
