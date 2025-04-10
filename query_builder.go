package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// --- Internal Query Builder ---

type queryBuilder struct {
	conditions []string
	args       []any
	phIndex    int //tracks next placeholder index ($1, $2, ...)
}

// newQueryBuilder creates a new builder instance.
func newQueryBuilder() *queryBuilder {
	return &queryBuilder{
		conditions: []string{},
		args:       []any{},
		phIndex:    1, // Start placeholders at $1
	}
}

// nextPlaceholder generates the next placeholder string (e.g., "$1") and increments the index.
func (qb *queryBuilder) nextPlaceholder() string {
	ph := fmt.Sprintf("$%d", qb.phIndex)
	qb.phIndex++
	return ph
}

// addCondition formats and adds a condition string and its arguments to the builder.
// It uses nextPlaceholder to manage indices automatically.
// Takes the SQL fragment (e.g., "field = %s", "field LIKE %s", "field BETWEEN %s AND %s")
// and the corresponding values.
func (qb *queryBuilder) addCondition(fragmentFormat string, values ...any) {
	placeholders := make([]any, len(values))
	for i := range values {
		placeholders[i] = qb.nextPlaceholder()
	}
	qb.conditions = append(qb.conditions, fmt.Sprintf(fragmentFormat, placeholders...))
	qb.args = append(qb.args, values...)
}

// addInCondition handles "field IN (...)" clauses specifically.
func (qb *queryBuilder) addInCondition(field string, values []string) {
	if len(values) == 0 {
		return
	}
	// Type conversion needed as values are strings, but args needs interface{}
	interfaceValues := make([]any, len(values))
	placeholders := make([]string, len(values))
	for i, v := range values {
		placeholders[i] = qb.nextPlaceholder()
		interfaceValues[i] = v // Store the original string value
	}

	condition := fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ", "))
	qb.conditions = append(qb.conditions, condition)
	qb.args = append(qb.args, interfaceValues...)
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
func applyFilters(qb *queryBuilder, params SearchParams) {
	fmt.Printf("%+v\n", params)
	if params.StartDate != "" && params.EndDate != "" {
		sd, errS := parseDateTime(params.StartDate)
		ed, errE := parseDateTime(params.EndDate)
		if errS == nil && errE == nil {
			// Note: %s is used here because nextPlaceholder() returns the full "$N" string.
			qb.addCondition("read_time BETWEEN %s AND %s", sd, ed)
		} else {
			log.Printf("Warning: Invalid date format provided, skipping date filter. StartErr: %v, EndErr: %v", errS, errE)
		}
	}

	if params.Geometry != nil {
		fmt.Printf("%+v\n", params.Geometry)

		//this should always work since we've already unmarshalled it
		//we should do something else because it's probably expensive to convert twice
		geoJSON, err := json.Marshal(params.Geometry)
		if err != nil {
			fmt.Printf("error marshalling geometry: %v", err)
		} else {
			//qb.addCondition("ST_Within(location, ST_SetSRID(ST_GeomFromText(%s), 4326))", params.Geometry)
			qb.addCondition("ST_Within(location, ST_SetSRID(ST_GeomFromGeoJSON(%s), 4326))", geoJSON)

		}

	}

	// Camera Names Filter (IN clause)
	//NOTE: This currently requires full camera names which might be too restrictive
	qb.addInCondition("camera_name", params.CameraNames)

	// State Filter
	if len(params.PlateCode) == 2 {
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

// --- Public Functions ---

// BuildSelectQuery constructs the SELECT query using the internal builder.
func BuildSelectQuery(jsonData []byte) (string, []any, error) {
	var params SearchParams
	err := json.Unmarshal(jsonData, &params)
	fmt.Printf("%+v\n", params)
	if err != nil {
		return "", nil, fmt.Errorf("invalid search document format: %w", err)
	}
	//TODO: if there is a faulty date format or missing date, consider setting default to today instead of returning an error
	if params.StartDate == "" || params.EndDate == "" {
		return "", nil, fmt.Errorf("start_date and end_date are required")
	}

	qb := newQueryBuilder()
	applyFilters(qb, params) // Use the shared filter logic

	// Base select statement
	baseSQL := `SELECT
        plate_num, plate_code, camera_name, read_id, read_time, image_id,
        ST_AsText(location) as location, make, vehicle_type, color,
        doc->'source'->>'id' as source_id FROM alpr`

	// Assemble final query
	sql := baseSQL + qb.whereClause() + " ORDER BY read_time DESC, id DESC"

	// Add pagination
	page := params.Page
	pageSize := params.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 1000
	}
	offset := (page - 1) * pageSize

	// Add limit/offset using the builder to manage placeholders and args
	// We create temporary placeholders just for formatting the SQL string fragment
	limitPh := qb.nextPlaceholder()
	offsetPh := qb.nextPlaceholder()
	sql += fmt.Sprintf(" LIMIT %s OFFSET %s", limitPh, offsetPh)
	args := append(qb.args, pageSize, offset) // Manually append pagination args

	return sql, args, nil
}

// BuildCountQuery constructs the COUNT query using the internal builder.
func BuildCountQuery(jsonData []byte) (string, []any, error) {
	var params SearchParams // Only needed for filtering
	err := json.Unmarshal(jsonData, &params)
	if err != nil {
		return "", nil, fmt.Errorf("invalid search document format: %w", err)
	}
	// No need to validate required fields strictly for count filters,
	// applyFilters will handle missing ones gracefully.

	qb := newQueryBuilder()
	applyFilters(qb, params) // Use the shared filter logic

	baseSQL := `SELECT count(id) FROM alpr`
	sql := baseSQL + qb.whereClause()

	return sql, qb.args, nil // Return SQL and args accumulated by the builder
}

// CalculateTotalPages remains the same
func CalculateTotalPages(totalCount int64, pageSize int) int {
	if totalCount <= 0 {
		return 0
	}
	effPageSize := pageSize
	if effPageSize <= 0 {
		effPageSize = 1000
	}
	return int(math.Ceil(float64(totalCount) / float64(effPageSize)))
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
