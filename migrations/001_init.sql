CREATE TABLE IF NOT EXISTS people (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS devices (
  id          TEXT PRIMARY KEY,
  person_id   TEXT REFERENCES people(id),
  label       TEXT NOT NULL,
  kind        TEXT NOT NULL,
  agent_token TEXT,
  state       TEXT NOT NULL,
  last_seen   TIMESTAMP,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_devices_state  ON devices(state);
CREATE INDEX IF NOT EXISTS idx_devices_person ON devices(person_id);

-- An identity value is globally unique across devices. Two devices claiming the
-- same MAC/agent_id is exactly the case Reconcile is meant to collapse.
CREATE TABLE IF NOT EXISTS network_identities (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id  TEXT NOT NULL REFERENCES devices(id),
  kind       TEXT NOT NULL,
  value      TEXT NOT NULL,
  last_seen  TIMESTAMP,
  UNIQUE(kind, value)
);
CREATE INDEX IF NOT EXISTS idx_identities_device ON network_identities(device_id);

CREATE TABLE IF NOT EXISTS device_capabilities (
  device_id  TEXT NOT NULL REFERENCES devices(id),
  capability TEXT NOT NULL,
  PRIMARY KEY (device_id, capability)
);
