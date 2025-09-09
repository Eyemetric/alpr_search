# ALPR SERVICE
This is a brief overview of the ALPR service.

A middleware service that connects PlateSmart license plate recognition service and NJSNAP.
It provides an API with the following endpoints:

- /Add: accepts License plate records for storage up to 3yrs
- /Search:  a search api for searching license plate data and images with robust search capabilities such as:
  - partial license plate data.
  - searching within a given area via geojson
  - date/time filtering
  - other vehicle attributes like (color, make , model)

- /Hotlist: allows NJSNAP to ADD|EDIT|DELETE POI items that will be used to alert the state when a vehicle with a license plate matching the BOLO is detected. In the case of Delete, that will remove the item from the hotlist.

## Phase 1 Storage and Search

### /Add POST an endpoint for PlateSmart license plate data.

Platesmart posts json data to the endpoint representing a detected license plate. It is a constant stream of requests.
It never ends, 24/7 365

### /Search POST

Search is a single endpoint allowing POST requests. A json document representing a search request is sent to the enpoint where it is converted to an sql query and a json document containing the results are returned.
Search results are paginated and offer a maximum of 1000 results per page.

#### Search Document Example
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

#### Search Results example

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

## Phase 2:  Person of Interest (POI)

This feature allows NJSnap add and remove vehicles from the POI hotlist. When a vehicle is entered into the system, it is checked against the POI hotlist. If the vehicle is found on the hotlist, an alert is sent to the NJSNAP system.

 There are 3 types of hotlists:
 1. NJSnap: The most urgent and recent BOLOs sent from the state to our endpoint
 2. DMV: Daily bolo list updates once daily in the mornings
 3. NCIC: Weekly BOLO from National Crime Information Center (NCIC)


### /Hotlist POST

```json



```
