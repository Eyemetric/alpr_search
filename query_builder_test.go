package main

import (
	"encoding/json"
	"log"
	"testing"
)

func TestBuildSelectQuery(t *testing.T) {

	jdata := `{
		"page": 1,
		"page_size": 500,
		"start_date": "2025-01-01T00:00:00Z",
		"end_date": "2025-01-31T23:59:59Z",
		"plate_num": "%MG%" }`

	jdata = `
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
	var s_doc SearchDoc
	err := json.Unmarshal([]byte(jdata), &s_doc)
	if err != nil {
		log.Fatalf("couldn't parse json to search doc: %e", err)
	}
	s_query, err := BuildSelectQuery(s_doc)
	if err != nil {
		println("you done f'd up %v", err)
	}

	t.Logf("%s\n", s_query)
}

func TestParseDateTime(t *testing.T) {

	dts := "2024-05-01T12:00:01"
	dt, err := parseDateTime(dts)
	if err != nil {
		log.Fatalf("couldn't parse: %e", err)
	}
	log.Println(dt)
}

func TestFilterGeo(t *testing.T) {

}
