package search

/* QueryBuilder transforms a SearchDoc into a SQL Query.
SearchDoc is a struct that represents the client's search request

*/

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

type queryBuilder struct {
	conditions []string
	args       []any
	phIndex    int //tracks next placeholder index ($1, $2, ...)
}

func newQueryBuilder() *queryBuilder {
	return &queryBuilder{
		conditions: []string{},
		args:       []any{},
		phIndex:    1, // Start placeholders at $1
	}
}

// NOTE: RFC3339 is a stricter version of ISO8601
func parseDateTime(dateTimeStr string) (time.Time, error) {
	//if no date is passed, set to the current day?
	layouts := []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05"}
	var t time.Time
	var err error
	for _, layout := range layouts {
		if t, err = time.Parse(layout, dateTimeStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format for '%s': %w", dateTimeStr, err)
}

// nextPlaceholder generates the next placeholder string (e.g., "$1") and increments the index.
func (qb *queryBuilder) nextPlaceholder() string {
	ph := fmt.Sprintf("$%d", qb.phIndex)
	qb.phIndex++
	return ph
}

// addCondition formats and adds a condition string and its arguments to the builder.
// These become part of the WHERE clause
// It uses nextPlaceholder to manage indices automatically.
// Takes the SQL fragment (e.g., "field = %s", "field LIKE %s", "field BETWEEN %s AND %s")
// and the corresponding values.
func (qb *queryBuilder) addCondition(fragment string, values ...any) {
	placeholders := make([]any, len(values))
	for i := range values {
		placeholders[i] = qb.nextPlaceholder()
	}
	//conditions are the filter statements, args are the values that will replace the placeholder parameters
	//when the query is executed
	qb.conditions = append(qb.conditions, fmt.Sprintf(fragment, placeholders...))
	qb.args = append(qb.args, values...)
}

// addInCondition is similar to addCondition but handles "field IN (...)" clauses specifically.
func (qb *queryBuilder) addInCondition(field string, values []string) {
	if len(values) == 0 {
		return
	}
	// Type conversion needed as values are strings, but args needs interface{}
	interfaceVals := make([]any, len(values))
	placeholders := make([]string, len(values))
	for i, v := range values {
		placeholders[i] = qb.nextPlaceholder()
		interfaceVals[i] = v // Store the original string value
	}

	condition := fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
	qb.conditions = append(qb.conditions, condition)
	qb.args = append(qb.args, interfaceVals...)
}

// whereClause constructs the final WHERE clause string.
func (qb *queryBuilder) whereClause() string {
	if len(qb.conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(qb.conditions, " AND ")
}

// applyFilters adds all relevant filters to the queryBuilder based on SearchParams.
// This function encapsulates the repetitive filtering logic.
func (qb *queryBuilder) applyFilters(params SearchDoc) {

	sd, errS := parseDateTime(params.StartDate)
	ed, errE := parseDateTime(params.EndDate)
	if errS == nil && errE == nil {
		// Note: %s is used here because nextPlaceholder() returns the full "$N" string.
		qb.addCondition("read_time BETWEEN %s AND %s", sd, ed)
	} else {
		log.Printf("Warning: Invalid date format provided, skipping date filter. StartErr: %v, EndErr: %v", errS, errE)
	}

	if params.Geometry != nil {

		//NOTE: this should always work since we've already unmarshalled it, which would have failed and returned
		//prior to this call,
		//we should do something else because it's probably expensive to convert twice
		geoJSON, err := json.Marshal(params.Geometry)
		if err != nil {
			fmt.Printf("error marshalling geometry: %v", err)
		} else {
			qb.addCondition("ST_Within(location, ST_SetSRID(ST_GeomFromGeoJSON(%s), 4326))", geoJSON)
		}
	}

	// Camera Names Filter (IN clause)
	//NOTE: This currently requires full camera names which might be too restrictive
	qb.addInCondition("camera_name", params.CameraNames)

	// State Filter
	if len(params.PlateCode) == 2 {
		//plate code is a 2 char state but it's stored in the DB with US- prepended by platesmart
		pcode := "US-" + strings.ToUpper(params.PlateCode)
		qb.addCondition("plate_code = %s", pcode)
	}

	// Vehicle Make Filter
	if len(params.Make) > 1 {
		qb.addCondition("LOWER(make) = LOWER(%s)", params.Make)
	}

	// Vehicle Type Filter
	if len(params.VehicleType) > 1 {
		qb.addCondition("LOWER(vehicle_type) = LOWER(%s)", params.VehicleType)
	}

	// Vehicle Color Filter
	if len(params.Color) > 1 {
		qb.addCondition("LOWER(color) = LOWER(%s)", params.Color)
	}

	// Plate Num Filter
	if params.PlateNum != "" {
		if strings.ContainsAny(params.PlateNum, "%_") {
			qb.addCondition("plate_num ILIKE %s", params.PlateNum)
		} else {
			qb.addCondition("plate_num = %s", params.PlateNum)
		}
	}
}

// Base select statement.
// NOTE: location is returned as a jsonb fragment so we don't need special golang GeomTypes, easier
const baseSQL = `
	    SELECT plate_num, plate_code, camera_name, read_id, read_time, image_id, make, vehicle_type, color,
	    CASE WHEN location IS NOT NULL THEN jsonb_build_object('lat', TRUNC(ST_Y(location)::numeric, 5), 'lon', TRUNC(ST_X(location)::numeric, 5))
	    ELSE jsonb_build_object('lat', 0.0, 'lon', 0.0)
	    END AS location, doc->'source'->>'id' as source_id FROM alpr`

// NOTE: this is limit offset style paging which may inhibit performance as the db size grows. The alternative is next_page tokens.
// which is faster but more limited in that only the next or previous page can be retrieved whereas limit/offset allows jumping to any page directly
func (qb *queryBuilder) addPagination(searchDoc SearchDoc) string {
	page := searchDoc.Page
	pageSize := searchDoc.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 1000
	}
	offset := (page - 1) * pageSize

	limitPh := qb.nextPlaceholder()
	offsetPh := qb.nextPlaceholder()
	qb.args = append(qb.args, pageSize, offset)
	order := " ORDER BY read_time DESC, id DESC"
	return fmt.Sprintf("%s LIMIT %s OFFSET %s", order, limitPh, offsetPh)
}

//Query struct

type Query struct {
	Text   string
	Params []any
}

// BuildSelectQuery constructs the SELECT query using the internal builder.
func BuildSelectQuery(searchDoc SearchDoc) (*Query, error) {

	//TODO: if there is a faulty date format or missing date, consider setting default to today instead of returning an error
	if searchDoc.StartDate == "" || searchDoc.EndDate == "" {
		return nil, fmt.Errorf("start_date and end_date are required")
	}

	qb := newQueryBuilder()
	qb.applyFilters(searchDoc) // Use the shared filter logic

	q := Query{}
	q.Text = fmt.Sprintf("%s %s %s", baseSQL, qb.whereClause(), qb.addPagination(searchDoc))
	q.Params = qb.args

	return &q, nil
}

// constructs the COUNT query using the internal builder.
// We need to get the total row count for a search so that we now how to divide the pages.
func BuildCountQuery(searchDoc SearchDoc) (*Query, error) {
	// No need to validate required fields strictly for count filters,
	// applyFilters will handle missing ones gracefully.

	qb := newQueryBuilder()
	qb.applyFilters(searchDoc) // Use the shared filter logic

	q := Query{}
	q.Text = fmt.Sprintf("Select count(*) from alpr %s", qb.whereClause())
	q.Params = qb.args

	return &q, nil

}

// CalculateTotalPages remains the same
func CalculateTotalPages(totalCount int64, pageSize int) int {
	if totalCount <= 0 {
		return 0
	}
	if pageSize <= 0 {
		pageSize = 1000
	}
	return int(math.Ceil(float64(totalCount) / float64(pageSize)))
}
