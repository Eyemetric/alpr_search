package search

import (
	"encoding/json"
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
	//Direction     string `json:"direction,omitempty"`
	//NextPageToken string `json:"next_page_token,omitempty"`
	//PrevPageToken string `json:"prev_page_token,omitempty"`
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

// the main struct that returns to caller as json
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
