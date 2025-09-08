![](/Users/ryan/Documents/eye_logo2.png){.center}


# Eyemetric ALPR API Documentation V1


## Overview
This API provides 2 endpoints for NJSnap :

1. Search: An api enpoint for searching detected vehicle info.
   Search records with criteria including
   - datetime ranges
   - full/partial license plates
   - vehicle characteristics (make, model , color )
   - geographic areas.

   The system returns paginated results and provides secure, temporary access to vehicle images.

2. Hotlist:  An api endpoint for adding new POI hotlist entries
   New plate reads coming from camera streams are checked against this list and anytime there is a match an alert will be sent.

## Authorization

You must add your api key as a Bearer Token to the Authorization Header for each api call.

# Search

## Building a Search Request

 A search request is represented as a  json object sent as a JSON body in a POST request. The API supports both required and optional search parameters.

To search for vehicle detections, send a POST request to the test api:

https://njalpr-hqhph3fbgrb0fgb5.westus-01.azurewebsites.net/api/alpr/v1/search


### Required Search Fields

Every search must include at least these basic fields:
```json
{
  "page": 1,                               // Starting page number
  "page_size": 20,                         // Results per page (20-1000)
  "start_date": "2024-02-01T00:00:00",    // Search start time
  "end_date": "2024-02-01T23:59:59",      // Search end time
  "plate_num": "%"                         // Plate number pattern
}
```

To get the next or previous page of results, simply pass the page number you want. 1 for page 1. 2 for page 2, etc.

**NOTE**: page_size can be limited to 20, 50, 100, 200, 500, or 1000


### Optional Search Fields

Additional fields can be included to refine your search. These can be omitted entirely or included with empty values:

```json
{
  "plate_code": "",        // 2 character State/jurisdiction code (NJ, FL , etc)
  "make": "",             // Vehicle manufacturer
  "vehicle_type": "",     // Type of vehicle
  "color": "",            // Vehicle color
  "camera_names": [       // Array of specific cameras to search
    "Mt Pleasant (Eastbound)",
    "Route 10 East River road right"
  ]
}
```
### License Plate Search Features

The `plate_num` field supports flexible searching using the '%' wildcard character:
- `%` by itself matches any plate number
- `ABC%` finds plates starting with "ABC"
- `%123` finds plates ending with "123"
- `A%B%C` finds plates starting with "A", containing "B", and ending with "C"

### Camera Names Feature

The `camera_names` field allows filtering by specific cameras:
- Accepts an array of camera name strings
- Camera names must match exactly (case sensitive)
- Partial matches on camera names are not supported yet
- Field can be omitted if not filtering by camera

Example:
```json
{
  "camera_names": [
    "Mt Pleasant (Eastbound)",
    "Route 10 East River road right"
  ]
}
```

### Geographic Search Feature

The API supports geographic searching through an optional `geometry` field using the GeoJSON Polygon format. This allows you to search for detections within a specific area.

```json
{
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
```

When using the geometry field, remember:

- Coordinates must be in [longitude, latitude] order
- The shape must be closed (first and last points identical)
- There is no limit on the complexity of the polygon shape or number of points.
- Only a single area per search request is currently supported. Try not to  send multiple distinct areas to search within a single request.

## Response Format and Pagination

The API returns paginated results with metadata about the total number of pages.

There are 2 fields returned in a search result. a metadata field for info about the results and a
results field containing the array of items up to the page_size.

### Metadata Behavior
- Currently page_count is only field returned as meta data.
- When requesting page 1 (the initial search), the response includes the total `page_count`
- Subsequent page requests (i.e next page) return `page_count` as -1. This is an optimization and indicates that the page count per a given page size is only calculated when the first page is requested and not when jumping to or cycling through pages.

### Image Access via Pre-signed URLs

Vehicle and plate images are stored in Wasabi Storage (S3-compatible object store) and are accessed through pre-signed URLs.

- Presigned Urls Provide temporary, direct access to images without requiring credentials
- Are valid for a limited time (typically 24 hours)
- Include authentication information in the URL
- Allow direct image access from the object store

Example Response:
```json
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
```

## Example Searches

### Basic Search with Specific Cameras
```json
{
  "page": 1,
  "page_size": 20,
  "start_date": "2024-12-20T00:00:00",
  "end_date": "2024-12-14T23:59:59",
  "plate_num": "%",
  "camera_names": [
    "Mt Pleasant (Eastbound)",
    "Route 10 East River road right"
  ]
}
```

### Comprehensive Search
```json
{
  "page": 1,
  "page_size": 50,
  "start_date": "2024-02-11T00:00:00",
  "end_date": "2024-02-14T23:59:59",
  "plate_num": "A%C%",
  "plate_code": "NY",
  "make": "Toyota",
  "vehicle_type": "Sedan",
  "color": "Blue",
  "camera_names": ["Mt Pleasant (Eastbound)"],
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
```

### A working example using Curl

Remember to replace the ${api_key} with the real api key, (provided separately).

```bash
curl -X POST "https://njalpr-hqhph3fbgrb0fgb5.westus-01.azurewebsites.net/api/alpr/v1/search/" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer ${api_key}"
-d '{
  "page": 1,
  "page_size": 20,
  "start_date": "2025-01-01T00:00:00",
  "end_date": "2025-01-02T23:59:59",
  "plate_num": "A%"
}'
```



## Important Guidelines
- Only page, page_size, start_date, end_date, and plate_num are required
- All other fields are optional and can be omitted from search request
- Camera names must match exactly (case sensitive). Temp limitation
- Start date must be earlier than end date
- Plate number searches must include at least one "%" character. Must not be empty
- All dates must be in ISO 8601 format
- Image URLs expire after their specified lifetime
- Search is case-insensitive (except for camera names)
- Consider date range scope in relation to search criteria:
  - Broad searches (like plate_num: "A%") should use narrower date ranges
  - Example: searching for "A%" over a full year with no other criteria is discouraged but allowed
  - More specific searches (exact plate, specific cameras, etc.) can use wider date ranges
  - Best practice: start with narrow date ranges and expand as needed based on your search criteria specificity

## Error Responses
The API uses standard HTTP status codes and provides detailed error messages:
```json
{
  "error": {
    "code": "INVALID_SEARCH",
    "message": "Invalid date range specified",
    "details": "End date must be after start date"
  }
}
```

# Hotlist

Adding hotlist entries using the Eyemetric hotlist api follows the spec layed out in the *NJ SNAP POI API Documentation_revision_3e Final*
document.


## Building a Hotlist Request

 A hotlist is represented as a json object containing a POI array of 1 or more hotlist entries and sent via POST request. The format of the json document is expected to match the document provided in the POI API Documentation.

send a POST request to the test api:

https://njalpr-hqhph3fbgrb0fgb5.westus-01.azurewebsites.net/api/alpr/v1/hotlist

### A working example using curl

```bash
curl -X POST "https://njalpr-hqhph3fbgrb0fgb5.westus-01.azurewebsites.net/api/alpr/v1/hotlist/" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer ${api_key}"
-d '{
  "POIs": [
    {
      "ID": "4",
      "Status": "ADD",
      "StartDate": "2025-08-18T13:28:38.277",
      "ExpirationDate": "2025-08-30T13:28:38.277",
      "UserVisibility": "Y",
      "NJSNAPHitNotification": "Y",
      "ReasonType": "NON-NCIC",
      "Reason": "BOLO ALERT",
      "PlateNumber": "TEST1234",
      "PlateSt": "NJ",
      "NICNum": "TEST1234",
      "VehicleMake": "",
      "VehicleModel": "",
      "VehicleYear": "",
      "VehicleColor": "",
      "VehicleSize": "",
      "ContactName": "Manager Furst, Last name",
      "ContactNo": "764-474-4483",
      "ContactEmail": "kiran@gtbm.com",
      "CaseNo": "WE3456",
      "Comment": "",
      "DeptORI": "NJTEST4567",
      "DeptName": "CENTRALTEST1",
      "EntryDate": "2025-08-13T10:08:10.37"
    }
  ]
}'
```


# Plate Hit Alerts

Anytime a plate read is received it is checked against any existing and valid hotlist entries. If a plate match is detected and alert will immediately be pushed to NJSnap.

The plate hit document sent as an alert follows the POI API Documentation.

```json

{
  "plateHits": [
    {
      "ID": "12",
      "eventID": "TEST3456",
      "eventDateTime": "2022-12-13 10:09:20",
      "plateNumber": "TEST3456",
      "plateSt": "NJ",
      "plateNumber2": "",
      "confidence": "",
      "vehicleMake": "BMW",
      "vehicleModel": "X5",
      "vehicleColor": "White",
      "vehicleSize": "Medium",
      "vehicleType": "SUV",
      "cameraID": "2345",
      "cameraName": "Camera Testing 3456",
      "cameraType": "Fixed",
      "agency": "Newark",
      "ori": "NJ3454677",
      "latitude": 39.57945694,
      "longitude": -74.756544,
      "direction": "North",
      "imageVehicle": "A presigned url",
      "imagePlate": "A presigned url",
      "additionalImage1": "",
      "additionalImage2": ""
    }
  ]
}

```

Note: imageVehicle and imagePlate are not sent as base64 encoded text but as a secure presigned url to the images. This is the same style link as described by the search endpoint.

## Plate Hit Alert failure and recover strategy

In the event of a failed send due to NJSnap being unavailable or returning a non 200 response code,  the Eyemetric alpr service will enter into a retry state as proposed in the *Addendum for Handling NJ SNAP POI Hists and Entries* doc.

## Proposed New Strategy (Vendors to NJ SNAP)
* Every 20 seconds/3Xs = 1 min.
* Every 60 seconds/4xs = 4 min.
* Mark that it was not able to send it to the vendor.
* Track POIs that failed to send, move into a QUEUE
* Poll API every hour for 3 hours, 20 seconds/3Xs = 1 min.
* If still no 200 success, notify vendor(s) that NJ SNAP POI API is down.
* Continue to queue responses and attempt to send once per hour
* Once 200 success, send all POIs that are in que to the Vendor




Prepared by:

Ryan Martin

ryan@eyemetric.com

(720) 692-0403
