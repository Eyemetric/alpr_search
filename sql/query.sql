-- name: IngestALPR :one
select public.ingest_alpr(@doc::jsonb) as result;

-- name: InsertHotlist :one
select hotlists_upsert_pois(@doc::jsonb) as added;


-- name: ClaimDue :many
with cte as (
   select id from alerts
  where status in ('pending', 'queued') and visible_at <= now()
  order by created_at
  for update skip locked
  limit sqlc.arg(batch)
)
update alerts a
set status='processing',
    locked_at=now(),
    locked_by=sqlc.arg(worker_id),
    processing_deadline=now() + interval '30 seconds'
from cte
where a.id = cte.id
returning a.id, a.plate_id, a.hotlist_id;


-- name: NextWake :one
select least(
    coalesce((select min(visible_at) as wake from alerts
             where status in ('pending', 'queued') and visble_at > now()), 'infinity'),
            (select next_due_at from hotlist_alert_state where id=1));


-- name: ScheduleSuccess :exec
select hotlist_alert_schedule_success(sqlc.arg(id));
-- name: ScheduleFailure :exec
select hostlist_alert_schedure_failure(sqlc.arg(id), sqlc.arg(err));

-- name: ReclaimStuck :one
select alerts_reclaim_stuck();
