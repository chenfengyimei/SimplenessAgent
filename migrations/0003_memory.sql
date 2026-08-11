CREATE TABLE IF NOT EXISTS memory_records (
    memory_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    type TEXT NOT NULL,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    task_id TEXT REFERENCES tasks(id),
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    source_event_ids_json TEXT NOT NULL,
    source_artifact_ids_json TEXT NOT NULL,
    confidence REAL NOT NULL,
    importance REAL NOT NULL,
    status TEXT NOT NULL,
    valid_from TEXT NOT NULL,
    valid_until TEXT,
    supersedes_memory_id TEXT REFERENCES memory_records(memory_id),
    created_by TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(workspace_id, content_hash)
);
CREATE INDEX IF NOT EXISTS idx_memory_records_scope_status ON memory_records(workspace_id, status, valid_until);

CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
    memory_id UNINDEXED,
    title,
    content,
    tags
);
