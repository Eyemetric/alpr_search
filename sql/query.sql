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
