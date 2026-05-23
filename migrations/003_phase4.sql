-- Phase 4: person-scoped policies + per-device overrides. Each row stores
-- structured intent as a JSON blob; the resolver flattens it at query time.

CREATE TABLE IF NOT EXISTS policies (
  person_id  TEXT PRIMARY KEY REFERENCES people(id),
  data       TEXT NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS device_overrides (
  device_id  TEXT PRIMARY KEY REFERENCES devices(id),
  data       TEXT NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
