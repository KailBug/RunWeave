CREATE TABLE store_identity (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    role TEXT NOT NULL CHECK (role = 'node'),
    instance_id TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    last_checked_at TEXT NOT NULL
);
