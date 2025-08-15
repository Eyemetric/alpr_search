-- 1) Extensions (ensure shared_preload_libraries includes 'timescaledb' on the server)
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS timescaledb_toolkit;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 2) Base table + sequence
CREATE TABLE IF NOT EXISTS public.alpr (
  id           bigint PRIMARY KEY,                 -- PK will be amended below to (id, read_time)
  doc          jsonb,
  inserted_at  timestamp(0) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  plate_num    varchar,
  read_time    timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
  camera_name  varchar,
  plate_code   varchar,
  image_id     varchar,
  location     public.geometry(Point,4326) NOT NULL,
  read_id      varchar,
  make         varchar,
  vehicle_type varchar,
  color        varchar
);

-- If not already present:
CREATE SEQUENCE IF NOT EXISTS public.alpr_id_seq;
ALTER SEQUENCE public.alpr_id_seq OWNED BY public.alpr.id;
ALTER TABLE public.alpr ALTER COLUMN id SET DEFAULT nextval('public.alpr_id_seq');

-- 3) Make it a hypertable (7-day chunks inferred from your dump; adjust to taste)
SELECT create_hypertable('public.alpr', 'read_time', chunk_time_interval => INTERVAL '7 days');

-- 4) Correct PK to match your existing shape (parent only)
ALTER TABLE public.alpr DROP CONSTRAINT alpr_pkey;
ALTER TABLE public.alpr ADD CONSTRAINT alpr_pkey PRIMARY KEY (id, read_time);

-- 5) Parent indexes (these will auto-create on each chunk)
CREATE INDEX IF NOT EXISTS alpr_read_time_idx          ON public.alpr (read_time DESC); --timeseries
CREATE INDEX IF NOT EXISTS idx_camera_name             ON public.alpr (camera_name);
CREATE INDEX IF NOT EXISTS idx_location_gist           ON public.alpr USING gist (location); --geo index for location
CREATE INDEX IF NOT EXISTS idx_plate_num_trgm          ON public.alpr USING gin (plate_num gin_trgm_ops); --fuzzy search
CREATE INDEX IF NOT EXISTS idx_read_time_and_id_desc   ON public.alpr (read_time DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_read_time_camera_name   ON public.alpr (read_time DESC, camera_name);
CREATE INDEX IF NOT EXISTS idx_read_time_plate_num     ON public.alpr (read_time DESC, plate_num);

-- (Optional) Reapply your column comments
COMMENT ON COLUMN public.alpr.read_time IS 'the time that the alpr system read the plate from a camera';
COMMENT ON COLUMN public.alpr.image_id IS 'used to build url to direct image access from S3';
COMMENT ON COLUMN public.alpr.read_id  IS 'unique id of the read scan';
COMMENT ON COLUMN public.alpr.vehicle_type IS 'sedan, suv, etc';

------ ALPR INGEST STRATEGY ----------

-- =========================================================
-- 1) Utility functions (centralize parsing & validation)
-- =========================================================
CREATE SCHEMA IF NOT EXISTS alpr_util;

-- Safe text getter: NULL if missing/blank
CREATE OR REPLACE FUNCTION alpr_util.jtext(doc jsonb, key text)
RETURNS text
LANGUAGE sql IMMUTABLE AS $$
  SELECT NULLIF(btrim(doc->>key), '')
$$;

-- Timestamp parser: ISO8601 string or numeric epoch seconds
CREATE OR REPLACE FUNCTION alpr_util.parse_ts(val jsonb)
RETURNS timestamptz
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE v timestamptz;
BEGIN
  IF val IS NULL THEN RETURN NULL; END IF;

  IF jsonb_typeof(val) = 'string' THEN
    BEGIN
      v := (val #>> '{}')::timestamptz;
      RETURN v;
    EXCEPTION WHEN OTHERS THEN
      RETURN NULL; -- invalid string
    END;
  ELSIF jsonb_typeof(val) = 'number' THEN
    RETURN to_timestamp((val #>> '{}')::float8);
  END IF;
  RETURN NULL;
END $$;

-- Geometry builder: supports {lon,lat}, [lon,lat], or "lon,lat"
CREATE OR REPLACE FUNCTION alpr_util.location_from_json(val jsonb)
RETURNS geometry(Point,4326)
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE lon float8; lat float8;
BEGIN
  IF val IS NULL THEN RETURN NULL; END IF;

  IF jsonb_typeof(val) = 'object' THEN
    lon := (val->>'lon')::float8; lat := (val->>'lat')::float8;
  ELSIF jsonb_typeof(val) = 'array' THEN
    lon := (val->>0)::float8;     lat := (val->>1)::float8;
  ELSE
    lon := split_part(val #>> '{}', ',', 1)::float8;
    lat := split_part(val #>> '{}', ',', 2)::float8;
  END IF;

  RETURN ST_SetSRID(ST_MakePoint(lon, lat), 4326);
END $$;

-- Optional helper: enforce nonblank with a clear error
CREATE OR REPLACE FUNCTION alpr_util.require_nonblank(val text, field text)
RETURNS text
LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
  IF val IS NULL OR btrim(val) = '' THEN
    RAISE EXCEPTION USING errcode='23514', message = field || ' required';
  END IF;
  RETURN val;
END $$;

-- =========================================================
-- 2) Staging table (isolated from the hypertable)
-- =========================================================
CREATE TABLE IF NOT EXISTS public.alpr_ingest (
  id           BIGSERIAL PRIMARY KEY,
  doc          JSONB NOT NULL,

  -- derived/typed fields (filled in trigger)
  plate_num    TEXT,
  read_time    TIMESTAMPTZ,
  camera_name  TEXT,
  plate_code   TEXT,
  image_id     TEXT,
  location     geometry(Point, 4326),
  read_id      TEXT,
  make         TEXT,
  vehicle_type TEXT,
  color        TEXT,

  inserted_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Optional for staging lookups
CREATE INDEX IF NOT EXISTS alpr_ingest_readid_idx ON public.alpr_ingest (read_id);
CREATE INDEX IF NOT EXISTS alpr_ingest_loc_gist   ON public.alpr_ingest USING gist (location);

-- =========================================================
-- 3) Tiny trigger using helpers
-- =========================================================
CREATE OR REPLACE FUNCTION public.alpr_ingest_fill()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
  IF jsonb_typeof(NEW.doc) <> 'object' THEN
    RAISE EXCEPTION USING errcode='22023', message='doc must be a JSON object';
  END IF;

  NEW.plate_num    := COALESCE(NEW.plate_num,    alpr_util.jtext(NEW.doc, 'plate_num'));
  NEW.camera_name  := COALESCE(NEW.camera_name,  alpr_util.jtext(NEW.doc, 'camera_name'));
  NEW.plate_code   := COALESCE(NEW.plate_code,   alpr_util.jtext(NEW.doc, 'plate_code'));
  NEW.image_id     := COALESCE(NEW.image_id,     alpr_util.jtext(NEW.doc, 'image_id'));
  NEW.read_id      := COALESCE(NEW.read_id,      alpr_util.jtext(NEW.doc, 'read_id'));
  NEW.make         := COALESCE(NEW.make,         alpr_util.jtext(NEW.doc, 'make'));
  NEW.vehicle_type := COALESCE(NEW.vehicle_type, alpr_util.jtext(NEW.doc, 'vehicle_type'));
  NEW.color        := COALESCE(NEW.color,        alpr_util.jtext(NEW.doc, 'color'));

  IF NEW.read_time IS NULL THEN
    NEW.read_time := alpr_util.parse_ts(NEW.doc->'read_time');
  END IF;

  IF NEW.location IS NULL THEN
    NEW.location := alpr_util.location_from_json(NEW.doc->'location');
  END IF;

  RETURN NEW;
END $$;

-- call ingest_fill() from trigger before insert or update
DROP TRIGGER IF EXISTS alpr_ingest_biu ON public.alpr_ingest;
CREATE TRIGGER alpr_ingest_biu
BEFORE INSERT OR UPDATE ON public.alpr_ingest
FOR EACH ROW EXECUTE FUNCTION public.alpr_ingest_fill();

-- 4) Constraints (idempotent via DO-block checks)
-- can't IF NOT EXISTS directly on a Contraint so we got to do it this way
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='doc_is_object' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT doc_is_object CHECK (jsonb_typeof(doc)='object');
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='plate_num_required' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT plate_num_required
      CHECK (COALESCE(plate_num, doc->>'plate_num') IS NOT NULL
             AND btrim(COALESCE(plate_num, doc->>'plate_num')) <> '');
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='read_time_shape_ok' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT read_time_shape_ok
      CHECK (doc ? 'read_time' AND jsonb_typeof(doc->'read_time') IN ('string','number'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='read_time_required' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT read_time_required CHECK (read_time IS NOT NULL);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='location_required' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT location_required CHECK (location IS NOT NULL);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='location_srid_4326' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT location_srid_4326 CHECK (st_srid(location)=4326);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='lat_valid' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT lat_valid CHECK (st_y(location) BETWEEN -90 AND 90);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='lon_valid' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT lon_valid CHECK (st_x(location) BETWEEN -180 AND 180);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='camera_name_not_blank' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT camera_name_not_blank
      CHECK (camera_name IS NULL OR btrim(camera_name) <> '');
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='plate_code_not_blank' AND conrelid='public.alpr_ingest'::regclass
  ) THEN
    ALTER TABLE public.alpr_ingest
      ADD CONSTRAINT plate_code_not_blank
      CHECK (plate_code IS NULL OR btrim(plate_code) <> '');
  END IF;
END $$;

-- 5) Dead-letter table. When json is invalid or inserts into alpr table fail.
-- we can use this to track failed inserts and retry them later.
-- one of the goals in this project is to lose nothing and recover state manually or automatically.
CREATE TABLE IF NOT EXISTS public.alpr_deadletter (
  id        BIGSERIAL PRIMARY KEY,
  failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  stage     TEXT        NOT NULL,     -- 'staging' | 'alpr-insert'
  sqlstate  TEXT,
  message   TEXT,
  detail    TEXT,
  hint      TEXT,
  context   TEXT,
  doc       JSONB       NOT NULL
);

CREATE INDEX IF NOT EXISTS alpr_deadletter_failed_at_idx
  ON public.alpr_deadletter (failed_at DESC);

-- 6) Entrypoint for staging, extenal programs call this
CREATE OR REPLACE FUNCTION public.ingest_alpr(p_doc JSONB)
RETURNS TEXT LANGUAGE plpgsql SECURITY DEFINER AS $$
DECLARE
  v_id BIGINT;
  v_sqlstate TEXT; v_msg TEXT; v_detail TEXT; v_hint TEXT; v_ctx TEXT;
BEGIN
  BEGIN
    INSERT INTO public.alpr_ingest(doc) VALUES (p_doc) RETURNING id INTO v_id;
  EXCEPTION WHEN OTHERS THEN
    GET STACKED DIAGNOSTICS v_sqlstate = returned_sqlstate,
                            v_msg      = message_text,
                            v_detail   = pg_exception_detail,
                            v_hint     = pg_exception_hint,
                            v_ctx      = pg_exception_context;
    INSERT INTO public.alpr_deadletter(stage, sqlstate, message, detail, hint, context, doc)
    VALUES ('staging', v_sqlstate, v_msg, v_detail, v_hint, v_ctx, p_doc);
    RETURN 'deadletter:staging';
  END;

  BEGIN
    INSERT INTO public.alpr (
      doc, inserted_at, plate_num, read_time, camera_name, plate_code,
      image_id, location, read_id, make, vehicle_type, color
    )
    SELECT
      doc, now(), plate_num, read_time, camera_name, plate_code,
      image_id, location, read_id, make, vehicle_type, color
    FROM public.alpr_ingest WHERE id = v_id;

    -- NOT Sure about this here
    DELETE FROM public.alpr_ingest WHERE id = v_id;
    RETURN 'ok';

  EXCEPTION WHEN OTHERS THEN
    GET STACKED DIAGNOSTICS v_sqlstate = returned_sqlstate,
                            v_msg      = message_text,
                            v_detail   = pg_exception_detail,
                            v_hint     = pg_exception_hint,
                            v_ctx      = pg_exception_context;
    INSERT INTO public.alpr_deadletter(stage, sqlstate, message, detail, hint, context, doc)
    SELECT 'alpr-insert', v_sqlstate, v_msg, v_detail, v_hint, v_ctx, doc
    FROM public.alpr_ingest WHERE id = v_id;
    RETURN 'deadletter:alpr-insert';
  END;

  -- I think this should be where the alert goes, if we're here we know the alpr insert succeeded

END $$;

-- we could call this function from an external process, or manually from a db program like psql
CREATE OR REPLACE FUNCTION public.reprocess_deadletter(p_id BIGINT)
RETURNS TEXT LANGUAGE plpgsql AS $$
DECLARE v_doc JSONB; v_res TEXT;
BEGIN
  SELECT doc INTO v_doc FROM public.alpr_deadletter WHERE id = p_id;
  IF v_doc IS NULL THEN RETURN 'not_found'; END IF;
  SELECT public.ingest_alpr(v_doc) INTO v_res;
  RETURN v_res;
END $$;


--- HOTLIST and queue implementation ---
-- =========================
-- Enums
-- =========================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'alert_status') THEN
    CREATE TYPE alert_status AS ENUM ('pending','processing','queued','done','failed','dead');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'hotlist_alert_mode') THEN
    CREATE TYPE hotlist_alert_mode AS ENUM ('normal','p0_fast','p1_minutely','p2_hourly_burst','p3_hourly_single');
  END IF;
END$$;

-- =========================
-- Core data tables
-- =========================
-- HOTLISTS: one row per POI from inbound JSON. Keep raw doc + extracted fields.
-- Note: external POI "ID" is stored in hotlist_id (text) to avoid clashing with PK id.
create table if not exists hotlists (
  id                       bigserial primary key,
  hotlist_id               text not null unique,          -- external POI ID (e.g., "4")
  status                   text not null,                 -- e.g., ADD
  start_date               timestamptz,                   -- e.g., 2022-12-13T13:28:38.277
  expiration_date          timestamptz,
  reason_type              text,
  plate_number             text not null,
  njsnap_hit_notification  boolean,
  doc                      jsonb not null,                -- full POI object
  created_at               timestamptz not null default now(),
  updated_at               timestamptz not null default now()
);

create index if not exists idx_hotlists_plate on hotlists (plate_number);
create index if not exists idx_hotlists_status on hotlists (status);
create index if not exists idx_hotlists_njsnap on hotlists (njsnap_hit_notification);
create index if not exists idx_hotlists_expiration on hotlists (expiration_date);
create index if not exists idx_hotlists_doc_gin on hotlists using gin (doc jsonb_path_ops);

-- touch updated_at
create or replace function set_updated_at()
returns trigger language plpgsql as $$
begin
  new.updated_at := now();
  return new;
end$$;

drop trigger if exists trg_hotlists_updated_at on hotlists;
create trigger trg_hotlists_updated_at
before update on hotlists
for each row execute function set_updated_at();

-- Core queue table (references hotlists)
create table if not exists alerts (
  id                   bigserial primary key,
  plate_id             bigint not null,
  hotlist_id           bigint not null references hotlists(id) on delete restrict,
  created_at           timestamptz not null default now(),
  status               alert_status not null default 'pending',
  attempts             int not null default 0,
  last_error           text,
  locked_at            timestamptz,
  locked_by            text,
  processing_deadline  timestamptz,
  visible_at           timestamptz not null default now()
);

create index if not exists idx_alerts_status_visible_at
  on alerts (status, visible_at)
  where status in ('pending','queued');

create index if not exists idx_alerts_created_at on alerts (created_at);
create index if not exists idx_alerts_hotlist_id on alerts (hotlist_id);

-- =========================
-- Ops events (optional)
-- TODO: This may not be necessary, but it's a good idea to have a way to track events related to alerts.
-- =========================
create table if not exists hotlist_alert_events (
  id bigserial primary key,
  kind text not null,
  created_at timestamptz not null default now(),
  details jsonb
);

-- =========================
-- Global scheduler (singleton row id=1)
-- =========================
create table if not exists hotlist_alert_state (
  id int primary key check (id=1),
  mode hotlist_alert_mode not null default 'normal',
  phase_attempts int not null default 0,
  first_failed_at timestamptz,
  vendor_down_notified_at timestamptz,
  next_due_at timestamptz not null default now()
);
insert into hotlist_alert_state(id) values (1) on conflict (id) do nothing;
create index if not exists idx_hotlist_alert_state_next_due_at on hotlist_alert_state (next_due_at);

-- =========================
-- Helpers
-- TODO: move to utils?
-- =========================
create or replace function next_hour(t timestamptz)
returns timestamptz language sql immutable as
$$ select date_trunc('hour', t) + interval '1 hour' $$;

-- Map 'Y'/'N' to boolean
create or replace function yn_bool(t text)
returns boolean language sql immutable as $$
  select case when t is null then null
              when upper(t) = 'Y' then true
              when upper(t) = 'N' then false
              else null end;
$$;

-- =========================
-- Align new alerts to current global schedule
-- We align alerts to hotlist state so all new records are on the same retry schedule if we're in a degraded mode
-- This is to prevent the "Thundering Herd" problem where many alerts are queued at the same time
-- =========================
create or replace function alerts_align_to_hotlist_state()
returns trigger language plpgsql as $$
declare m hotlist_alert_mode; nd timestamptz;
begin
  select mode, next_due_at into m, nd from hotlist_alert_state where id=1;
  if m = 'normal' then
    new.status := 'pending';
    new.visible_at := now();
  else
    new.status := 'queued';
    new.visible_at := nd;  -- herd into cadence in degraded modes
  end if;
  return new;
end$$;

drop trigger if exists trg_alerts_align on alerts;
create trigger trg_alerts_align
before insert on alerts
for each row execute function alerts_align_to_hotlist_state();

-- =========================
-- Notify workers on insert
-- =========================
create or replace function alerts_notify_insert()
returns trigger language plpgsql as $$
begin
  perform pg_notify('alerts_new', json_build_object('alert_id', new.id)::text);
  return new;
end$$;

-- an alert has been inserted, notify workers (ie the program responsible for pulling alert from queue to send)
drop trigger if exists trg_alerts_notify on alerts;
create trigger trg_alerts_notify
after insert on alerts
for each row execute function alerts_notify_insert();

-- =========================
-- Retry strategy proposed by NJSNAP (refer to docs for details)
-- Failure scheduler (global IF/ELSIF)
-- 20s×3 → 60s×4 → hourly bursts (hrs 2–4; 0s/20s/40s) → hourly single
-- Notifies 'vendor_down' once after 4 hours.
-- =========================
create or replace function hotlist_alert_schedule_failure(p_alert_id bigint, p_err text)
returns void language plpgsql as $$
declare
  nowh timestamptz := now();
  s hotlist_alert_state%rowtype;
  hours_since_first double precision;
begin
  perform pg_advisory_lock(42);
  select * into s from hotlist_alert_state where id=1 for update;

  if s.first_failed_at is null then
    s.first_failed_at := nowh;
  end if;

  if s.mode = 'normal' then
    s.mode := 'p0_fast';
    s.phase_attempts := 0;
    s.next_due_at := nowh + interval '20 seconds';

  elsif s.mode = 'p0_fast' and s.phase_attempts < 2 then
    s.phase_attempts := s.phase_attempts + 1;
    s.next_due_at := nowh + interval '20 seconds';

  elsif s.mode = 'p0_fast' then
    s.mode := 'p1_minutely';
    s.phase_attempts := 0;
    s.next_due_at := nowh + interval '60 seconds';

  elsif s.mode = 'p1_minutely' and s.phase_attempts < 3 then
    s.phase_attempts := s.phase_attempts + 1;
    s.next_due_at := nowh + interval '60 seconds';

  elsif s.mode = 'p1_minutely' then
    s.mode := 'p2_hourly_burst';
    s.phase_attempts := 0;
    s.next_due_at := next_hour(nowh);

  elsif s.mode = 'p2_hourly_burst' then
    hours_since_first := extract(epoch from (nowh - s.first_failed_at)) / 3600.0;

    if hours_since_first >= 4 and s.vendor_down_notified_at is null then
      insert into hotlist_alert_events(kind, details)
      values ('vendor_down', json_build_object('since', s.first_failed_at));
      s.vendor_down_notified_at := nowh;
    end if;

    if hours_since_first < 2 then
      s.next_due_at := next_hour(nowh);
    elsif hours_since_first <= 4 then
      if s.phase_attempts < 2 then
        s.phase_attempts := s.phase_attempts + 1;
        s.next_due_at := nowh + interval '20 seconds';
      else
        s.phase_attempts := 0;
        s.next_due_at := next_hour(nowh);
      end if;
    else
      s.mode := 'p3_hourly_single';
      s.phase_attempts := 0;
      s.next_due_at := next_hour(nowh);
    end if;

  elsif s.mode = 'p3_hourly_single' then
    s.next_due_at := next_hour(nowh);
  end if;

  update hotlist_alert_state set
    mode = s.mode,
    phase_attempts = s.phase_attempts,
    first_failed_at = s.first_failed_at,
    vendor_down_notified_at = s.vendor_down_notified_at,
    next_due_at = s.next_due_at
  where id = 1;

  update alerts
  set attempts = attempts + 1,
      last_error = p_err,
      status = case when s.mode = 'normal' then 'pending' else 'queued' end,
      visible_at = s.next_due_at,
      locked_by = null,
      locked_at = null,
      processing_deadline = null
  where id = p_alert_id;

  perform pg_advisory_unlock(42);
end$$;

-- =========================
-- Success handler (flip to normal + drain backlog)
-- =========================
create or replace function hotlist_alert_schedule_success(p_alert_id bigint)
returns void language plpgsql as $$
declare was_degraded boolean;
begin
  perform pg_advisory_lock(42);

  update alerts
  set status='done', locked_by=null, processing_deadline=null
  where id = p_alert_id;

  select (mode <> 'normal') into was_degraded from hotlist_alert_state where id=1;

  if was_degraded then
    insert into hotlist_alert_events(kind, details)
    values ('vendor_recovered', json_build_object('alert_id', p_alert_id));

    update hotlist_alert_state
    set mode='normal', phase_attempts=0, first_failed_at=null,
        vendor_down_notified_at=null, next_due_at=now()
    where id=1;

    update alerts
    set status='pending', visible_at=now()
    where status='queued';

    perform pg_notify('alerts_new', '{"bulk":"drain"}');
  end if;

  perform pg_advisory_unlock(42);
end$$;

-- =========================
-- (Optional) Reclaimer for stuck 'processing' rows
-- TODO: is this really needed? We might want to keep it around for debugging purposes.
-- =========================
create or replace function alerts_reclaim_stuck()
returns int language plpgsql as $$
declare n int;
begin
  update alerts
  set status='pending',
      locked_by=null,
      locked_at=null,
      processing_deadline=null,
      visible_at=now()
  where status='processing' and processing_deadline is not null and processing_deadline < now();
  get diagnostics n = row_count;
  if n > 0 then
    perform pg_notify('alerts_new', json_build_object('reclaimed', n)::text);
  end if;
  return n;
end$$;

-- =========================
-- Ingest helper: upsert every POI in a JSON payload
-- Usage: select hotlists_upsert_pois($$ {"POIs": [...]} $$::jsonb);
-- Returns: number of rows inserted/updated
-- NOTE: INSERTS new hotlist item, replaces existing hotlist item with the same hostlist_id
-- this means we have an EDIT hostlist
-- =========================
create or replace function hotlists_upsert_pois(p_doc jsonb)
returns int language plpgsql as $$
declare
  n int := 0;
  poi jsonb;
begin
  if p_doc ? 'POIs' then
    for poi in select * from jsonb_array_elements(p_doc->'POIs') loop
      insert into hotlists(
        hotlist_id, status, start_date, expiration_date,
        reason_type, plate_number, njsnap_hit_notification, doc
      )
      values (
        poi->>'ID',
        poi->>'Status',
        nullif(poi->>'StartDate','')::timestamptz,
        nullif(poi->>'ExpirationDate','')::timestamptz,
        poi->>'ReasonType',
        poi->>'PlateNumber',
        yn_bool(poi->>'NJSNAPHitNotification'),
        poi
      )
      on conflict (hotlist_id) do update set
        status = excluded.status,
        start_date = excluded.start_date,
        expiration_date = excluded.expiration_date,
        reason_type = excluded.reason_type,
        plate_number = excluded.plate_number,
        njsnap_hit_notification = excluded.njsnap_hit_notification,
        doc = excluded.doc,
        updated_at = now();
      n := n + 1;
    end loop;
  end if;
  return n;
end$$;
