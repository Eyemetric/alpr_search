-- name: IngestALPR :one
select alpr_util.ingest_alpr(@doc::jsonb) as result;

-- name: InsertHotlist :one
select alpr_util.hotlists_upsert_pois(@doc::jsonb) as added;

-- name: ClaimDue :many
SELECT
    t.id::bigint as id,
    t.plate_id::bigint as plate_id,
    t.hotlist_id::bigint as hotlist_id
FROM alpr_util.claim_due(@batch::integer , @worker_id::text) AS t(id bigint, plate_id bigint, hotlist_id bigint);



-- name: NextWake :one
select alpr_util.next_wake();
-- name: ScheduleSuccess :exec
select alpr_util.hotlist_alert_schedule_success(sqlc.arg(id));
-- name: ScheduleFailure :exec
select alpr_util.hostlist_alert_schedure_failure(sqlc.arg(id), sqlc.arg(err));

-- name: ReclaimStuck :one
select alpr_util.alerts_reclaim_stuck();

-- name: GetPlateHit :many
SELECT
    h.hotlist_id AS ID,
    'eyemetric' AS eventID,
    a.read_time AS eventDateTime,
    h.plate_number AS plateNumber,
    a.plate_code AS plateSt,
    '' AS plateNumber2,
    '' AS confidence,
    a.make AS vehicleMake,
    '' AS vehicleModel,
    a.color AS vehicleColor,
    '' AS vehicleSize,
    a.vehicle_type AS vehicleType,
    '' AS cameraID,
    a.camera_name AS cameraName,
    'Fixed' AS cameraType,
    'East Hanover Township Police Department' AS agency,
    '' AS ori,
    0.0 AS latitude,
    0.0 AS longitude,
    '' AS direction,
    '' AS imageVehicle,
    '' AS imagePlate,
    '' AS additionalImage1,
    '' AS additionalImage2
    FROM alpr a join hotlists h on a.plate_num = h.plate_number
  where a.id = @plate_id::bigint and h.id = @hotlist_id::bigint;
