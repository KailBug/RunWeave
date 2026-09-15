CREATE TABLE principals (
    id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL,
    revoked_at TEXT
);
CREATE TABLE node_identities (
    id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL,
    revoked_at TEXT
);
CREATE TABLE token_records (
    role TEXT NOT NULL CHECK (role IN ('principal', 'node')),
    principal_id TEXT UNIQUE REFERENCES principals(id),
    node_id TEXT UNIQUE REFERENCES node_identities(id),
    digest BLOB PRIMARY KEY CHECK (length(digest) = 32),
    created_at TEXT NOT NULL,
    revoked_at TEXT,
    CHECK ((role = 'principal' AND principal_id IS NOT NULL AND node_id IS NULL)
        OR (role = 'node' AND node_id IS NOT NULL AND principal_id IS NULL))
);
