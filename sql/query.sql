-- name: IngestALPR :one
select public.ingest_alpr(@doc::jsonb) as result;
