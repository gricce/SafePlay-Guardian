-- Phase 2: relax UNIQUE(kind, value) to UNIQUE(device_id, kind, value) so that
-- weak identities (IP via DHCP reassignment, generic hostnames, rotating
-- randomized MACs) can be recorded against multiple devices over time. Strong
-- identifiers (agent_id, non-randomized MAC) remain effectively unique because
-- the reconciler always merges into the existing owner before inserting.

CREATE TABLE network_identities_new (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id  TEXT NOT NULL REFERENCES devices(id),
  kind       TEXT NOT NULL,
  value      TEXT NOT NULL,
  last_seen  TIMESTAMP,
  UNIQUE(device_id, kind, value)
);

INSERT INTO network_identities_new (id, device_id, kind, value, last_seen)
SELECT id, device_id, kind, value, last_seen FROM network_identities;

DROP TABLE network_identities;
ALTER TABLE network_identities_new RENAME TO network_identities;

CREATE INDEX idx_identities_device ON network_identities(device_id);
CREATE INDEX idx_identities_kv     ON network_identities(kind, value);

-- Pending merges record reconciliations that produced multiple confident
-- candidates. The parent decides which (if any) to merge.
CREATE TABLE IF NOT EXISTS pending_merges (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  source_device_id TEXT NOT NULL REFERENCES devices(id),
  candidate_ids    TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending'
);
