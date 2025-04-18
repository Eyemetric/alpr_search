# ALPR SEARCH

Search for vehicles that have been captured by ALPR cameras using a simple API.

This api has a single api endpoint called search. A json document representing a search request is sent to the enpoint where it is 
converted to a sql query and a json document containing the results are returned. 

## Search Document Example 
```json
{
  "page": 1,
  "page_size": 1000,
  "start_date": "2025-03-01T00:00:00",
  "end_date": "2025-03-30T23:59:00",
  "plate_num": "A%B%",
  "plate_code": "NJ",
  "geometry": {
    "type": "Polygon",
    "coordinates": [[
      [
        -74.41513328481629,
        40.821574087857705
      ],
      [
        -74.36449324881904,
        40.82226769139721
      ],
      [
        -74.34753685667518,
        40.79365052431127
      ],
      [
        -74.37457542793163,
        40.769012536988306
      ],
      [
        -74.42040351480696,
        40.76883899762962
      ],
      [
        -74.44056787303212,
        40.79677299678702
      ],
      [
        -74.41513328481629,
        40.821574087857705
      ]
    ]]
  }
}

```

## Search Results example

```json
{
    "metadata": {
        "page_count": 7011
    },
    "results": [      
        {
            "plate_num": "A88UDR",
            "plate_code": "US-NJ",
            "camera_name": "Route 10 East River Road Right",
            "read_id": "5e00b710413846e09b327b7881165fb2",
            "read_time": "2025-04-18T21:47:43Z",
            "image_id": "570e447a40df4297a8787dc6f8f153bf",
            "location": {
                "lat": 40.80318,
                "lon": -74.36394
            },
            "make": "Toyota",
            "vehicle_type": "Sedan",
            "color": null,
            "source_id": "b5e7c19fdd0b4d97ac9c687d339621ec",
            "plate_img": "https://s3.wasabisys.com/njsnap/alpr-plate/b5e7c19fdd0b4d97ac9c687d339621ec",
            "full_img": "https://s3.wasabisys.com/njsnap/alpr/b5e7c19fdd0b4d97ac9c687d339621ec/12345567",
            "site_id": "NJ0141000",
            "user_id": null,
            "agency_name": "East Hanover Township Police Department"
        },
     
        {
            "plate_num": "A25RKE",
            "plate_code": "US-NJ",
            "camera_name": "Route 10 West River Road Left",
            "read_id": "916b1f97b4e048728443643cfd7f4003",
            "read_time": "2025-04-18T21:45:06Z",
            "image_id": "667c77c5d24648959eb03e173d91eda1",
            "location": {
                "lat": 40.80264,
                "lon": -74.36226
            },
            "make": "Ford",
            "vehicle_type": null,
            "color": null,
            "source_id": "8c1bfde1a9914d5c85546b3db0b1c913",
            "plate_img": "https://s3.wasabisys.com/njsnap/alpr-plate/b5e7c19fdd0b4d97ac9c687d339621ec",
            "full_img": "https://s3.wasabisys.com/njsnap/alpr/b5e7c19fdd0b4d97ac9c687d339621ec/12345567",
            "site_id": "NJ0141000",
            "user_id": null,
            "agency_name": "East Hanover Township Police Department"
        }
    ]
}
```
