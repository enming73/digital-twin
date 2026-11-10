CREATE TABLE runs (
  run_id text PRIMARY KEY,
  stations jsonb NOT NULL,
  integrity text NOT NULL,
  reason text NOT NULL DEFAULT '',
  last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0)
);
CREATE TABLE events (
  run_id text NOT NULL REFERENCES runs(run_id),
  event_id text NOT NULL,
  sequence bigint NOT NULL CHECK (sequence > 0),
  schema_version integer NOT NULL,
  sim_time_ms bigint NOT NULL CHECK (sim_time_ms >= 0),
  emitted_at timestamptz NOT NULL,
  ingested_at timestamptz NOT NULL,
  envelope jsonb NOT NULL,
  PRIMARY KEY (run_id, event_id),
  UNIQUE (run_id, sequence)
);
CREATE INDEX events_time_idx ON events(run_id, sim_time_ms, sequence);
CREATE TABLE snapshots (
  run_id text NOT NULL REFERENCES runs(run_id),
  sequence bigint NOT NULL,
  sim_time_ms bigint NOT NULL,
  format_version integer NOT NULL,
  reducer_version integer NOT NULL,
  checkpoint jsonb NOT NULL,
  PRIMARY KEY (run_id, sequence)
);
CREATE TABLE assets (
  machine_id text PRIMARY KEY,
  station_type text NOT NULL,
  representation text NOT NULL,
  model_url text
);
INSERT INTO assets (machine_id, station_type, representation, model_url) VALUES
  ('loader','loader','procedural',NULL), ('printer','printer','glb','/models/printer.glb'),
  ('spi','spi','procedural',NULL), ('mounter','mounter','glb','/models/mounter.glb'),
  ('reflow','reflow','procedural',NULL), ('aoi','aoi','procedural',NULL),
  ('unloader','unloader','procedural',NULL);
CREATE TABLE board_trace (
  run_id text NOT NULL REFERENCES runs(run_id),
  board_id text NOT NULL,
  revision bigint NOT NULL,
  projection jsonb NOT NULL,
  PRIMARY KEY (run_id, board_id)
);
CREATE TABLE work_order_projection (
  run_id text NOT NULL REFERENCES runs(run_id),
  work_order_id text NOT NULL,
  revision bigint NOT NULL,
  projection jsonb NOT NULL,
  PRIMARY KEY (run_id, work_order_id)
);
