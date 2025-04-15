package main

import (
	"testing"
)

func TestBuildSelectQuery(t *testing.T) {

	jdata := `{
		"page": 1,
		"page_size": 500,
		"start_date": "2025-01-01T00:00:00Z",
		"end_date": "2025-01-31T23:59:59Z",
		"plate_num": "%MG%" }`

	_ = `
	{
	  "page": 1,
	  "page_size": 20,
	  "start_date": "2024-02-01T00:00:00",
	  "end_date": "2024-02-01T23:59:59",
	  "plate_num": "ABC%",
	  "plate_code": "USA-NY",
	  "make": "Toyota",
	  "vehicle_type": "Sedan",
	  "color": "Blue",
	  "geometry": {
	    "type": "Polygon",
	    "coordinates": [[
	      [-74.5, 40.5],
	      [-74.3, 40.5],
	      [-74.3, 40.7],
	      [-74.5, 40.7],
	      [-74.5, 40.5]
	    ]]
	  }
	}
	`

	//TODO: unmarshal the json to SearchDoc
	s_query, _, err := BuildSelectQuery([]byte(jdata))
	if err != nil {
		println("you done f'd up %v", err)
	}

	t.Logf("%s\n", s_query)
}

func TestFilterGeo(t *testing.T) {

}
