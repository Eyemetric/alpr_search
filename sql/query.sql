-- name: IngestALPR :one
select alpr_util.ingest_alpr(@doc::jsonb) as result;

-- name: InsertHotlist :one
select alpr_util.hotlists_upsert_pois(@doc::jsonb) as added;
--
-- name: ScheduleSuccess :exec
select alpr_util.hotlist_alert_schedule_success(sqlc.arg(id));

-- name: ScheduleFailure :exec
select alpr_util.hotlist_alert_schedule_failure(sqlc.arg(id), sqlc.arg(err));

-- name: ReclaimStuck :one
select alpr_util.alerts_reclaim_stuck();

-- name: ClaimDue :many
select
    id::bigint as id,
    plate_id::bigint as plate_id,
    hotlist_id::bigint as hotlist_id
from alpr_util.claim_due(@batch::integer , @worker_id::text);


-- name: GetPlateHit :many
select
    h.hotlist_id as ID,
    'eyemetric' as eventID,
    a.read_time as eventDateTime,
    h.plate_number as plateNumber,
    a.plate_code as plateSt,
    '' as plateNumber2,
    '' as confidence,
    a.make as vehicleMake,
    '' as vehicleModel,
    a.color as vehicleColor,
    '' as vehicleSize,
    a.vehicle_type as vehicleType,
    '' as cameraID,
    a.camera_name as cameraName,
    'Fixed' as cameraType,
    'East Hanover Township Police Department' as agency,
    'NJ0141000' as ori,
    coalesce(ST_Y(location), 0) as latitude,
    coalesce(ST_X(location), 0) as longitude,
    '' as direction,
    '' as imageVehicle,
    '' as imagePlate,
    '' as additionalImage1,
    '' as additionalImage2,
    a.image_id,
    coalesce(a.doc->'source'->>'id', '') as source_id
    from alpr a join hotlists h on a.plate_num = h.plate_number
  where a.id = @plate_id::bigint and h.id = @hotlist_id::bigint;

  -- not using next_wake(). using a 5 sec. db poll. simpler
  -- name: NextWake :one
  select alpr_util.next_wake();
