-- Phase 6: append-only event log, parent auth, retention support.
--
-- Privacy posture (load-bearing, see CORE_PLATFORM.md):
--   * `events` is append-only. The retention job in internal/event deletes
--     rows older than the configured window; nothing else issues DELETEs.
--   * The effective monitoring level (resolved per person/device) determines
--     which fields of `attrs` get persisted in the first place. The schema
--     does not assume granular data.

CREATE TABLE IF NOT EXISTS events (
  id          TEXT PRIMARY KEY,
  at          TIMESTAMP NOT NULL,
  recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  kind        TEXT NOT NULL,
  person_id   TEXT,
  device_id   TEXT NOT NULL,
  severity    TEXT NOT NULL DEFAULT 'info',
  attrs       TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_events_person_kind_at ON events(person_id, kind, at);
CREATE INDEX IF NOT EXISTS idx_events_device_at      ON events(device_id, at);
CREATE INDEX IF NOT EXISTS idx_events_at             ON events(at);
CREATE INDEX IF NOT EXISTS idx_events_recorded_at    ON events(recorded_at);

CREATE TABLE IF NOT EXISTS auth_users (
  id         TEXT PRIMARY KEY,
  username   TEXT NOT NULL UNIQUE,
  pw_hash    TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  token      TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES auth_users(id),
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON auth_sessions(expires_at);
