package main

import (
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Or your chosen driver
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

type SearchParams struct {
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

type AlprRecord struct {
	PlateNum    string         `db:"plate_num"`
	PlateCode   sql.NullString `db:"plate_code"`
	CameraName  string         `db:"camera_name"`
	ReadID      int64          `db:"read_id"`
	ReadTime    time.Time      `db:"read_time"`
	ImageID     string         `db:"image_id"`
	Location    sql.NullString `db:"location"`
	Make        sql.NullString `db:"make"`
	VehicleType sql.NullString `db:"vehicle_type"`
	Color       sql.NullString `db:"color"`
	SourceID    sql.NullString `db:"source_id"`
}

func main() {

	jdata := `{
		"page": 1,
		"page_size": 500,
		"start_date": "2025-01-01T00:00:00Z",
		"end_date": "2025-01-31T23:59:59Z",
		"plate_num": "%MG%" }`

	s_query, _, err := BuildSelectQuery([]byte(jdata))
	if err != nil {
		println("you done f'd up %v", err)
	}

	println(s_query)
}
