CREATE TABLE IF NOT EXISTS probes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,
    kind        TEXT NOT NULL,
    target      TEXT,
    http_code   INTEGER,
    latency_ms  INTEGER,
    exit_ip     TEXT,
    ok          INTEGER NOT NULL,
    raw_error   TEXT
);
CREATE INDEX IF NOT EXISTS idx_probes_ts ON probes(ts);

CREATE TABLE IF NOT EXISTS incidents (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    trigger_reason  TEXT NOT NULL,
    initial_state   TEXT,
    terminal_state  TEXT,
    rotation_count  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS rotations (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id      INTEGER NOT NULL,
    started_at       INTEGER NOT NULL,
    ended_at         INTEGER,
    old_ip           TEXT,
    new_ip           TEXT,
    detection_method TEXT,
    ok               INTEGER,
    error            TEXT,
    FOREIGN KEY (incident_id) REFERENCES incidents(id)
);

CREATE TABLE IF NOT EXISTS notifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,
    incident_id INTEGER,
    level       TEXT NOT NULL,
    text        TEXT NOT NULL,
    buttons     TEXT,
    sent_at     INTEGER,
    error       TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS config_kv (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
