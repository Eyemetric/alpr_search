# Vehicle Detection System API Guide

## Overview
This API enables searching of vehicle detection records across multiple criteria including date time ranges, license plates, vehicle characteristics, and geographic areas. The system returns paginated results and provides secure, temporary access to vehicle images.

## Building a Search Request

To search for vehicle detections, send a POST request to `/api/search` with a JSON body containing your search criteria. While the API is flexible in what you can search for, it requires specific formatting of the request document.

### Required Search Fields
Every search must include these basic fields, when not actively used for filtering set the value to an empty string:

```json
{
  "page": 1,                               // Starting page number
  "page_size": 20,                         // Results per page (20-1000)
  "start_date": "2024-02-01T00:00:00",    // Search start time
  "end_date": "2024-02-01T23:59:59",      // Search end time
  "plate_num": "%",                        // Plate number pattern
  "plate_code": "",                        // State abbreviation
  "make": "",                              // Vehicle manufacturer
  "vehicle_type": "",                      // Type of vehicle
  "color": ""                              // Vehicle color
}
```

page_size can be set to: 
20, 50, 100, 200, 500, or 1000


### Geographic Search Capability

The API supports geographic searching through an optional `geometry` field using GeoJSON Polygon format. This allows you to search for detections within a specific area by defining a series of coordinates that form a closed shape.

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
- Coordinates should be listed counter-clockwise
- The geometry field can be completely omitted if not needed

## License Plate Search Features

The `plate_num` field supports flexible searching using the '%' wildcard character:
- `%` by itself matches any plate number
- `ABC%` finds plates starting with "ABC"
- `%123` finds plates ending with "123"
- `A%B%C` finds plates starting with "A", containing "B", and ending with "C"


## Response Format and Image Access

The resulting json response has contains 2 fields: 
- metadata : information about pagination
- results:  an array of json objects representing a matched vehicle
### Pre-signed URLs for Image Access
Vehicle and plate images are stored in Wasabi Storage (S3-compatible) and are accessed through pre-signed URLs. These URLs:
- Provide temporary, direct access to images without requiring credentials
- Are valid for a limited time (typically 24 hours)
- Include authentication information in the URL
- Allow direct image access without going through the API server

Example of a pre-signed URL:
```
https://bucket-name.s3.wasabisys.com/path/to/image.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20240214%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20240214T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=signature
```


### Response Structure

The json object returned from a Search 

```json
{
  "metadata": {
    "page": 1,
    "page_size": 20,
    "page_count": 50,
    "total_records": 1000
  },
  "results": [
    {
      "read_time": "2024-02-01T10:30:45Z",
      "camera_name": "CAM-123",
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
    // ... additional results
  ]
}
```

## Example Searches

### Basic Search (Any Plate)
```json
{
  "page": 1,
  "page_size": 20,
  "start_date": "2024-02-01T00:00:00",
  "end_date": "2024-02-01T23:59:59",
  "plate_num": "%",
  "plate_code": "",
  "make": "",
  "vehicle_type": "",
  "color": ""
}
```

### Comprehensive Search with Location
```json
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
```

## Important Guidelines
- Include all string fields, using empty strings ("") when not filtering
- The geometry field is the only optional field that can be omitted
- Start date must be earlier than end date
- Plate number searches must include at least one "%" character unless using a fully known plate number.
- All dates must be in ISO 8601 format (YYYY-MM-DDThh:mm:ss)
- Image URLs expire after their specified lifetime (currently set at 6hrs. ). Submitting a new search query will return new presigned urls with new lifetimes. 
- Search is case-insensitive

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