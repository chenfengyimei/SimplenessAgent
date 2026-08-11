PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    name TEXT NOT NULL,
    root_path TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    title TEXT NOT NULL,
    goal TEXT NOT NULL,
    status TEXT NOT NULL,
    spec_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_workspace_updated ON tasks(workspace_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS plan_versions (
    plan_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    revision INTEGER NOT NULL,
    parent_plan_id TEXT,
    reason TEXT NOT NULL,
    summary TEXT NOT NULL,
    plan_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(task_id, revision)
);

CREATE TABLE IF NOT EXISTS steps (
    step_id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL REFERENCES plan_versions(plan_id),
    task_id TEXT NOT NULL REFERENCES tasks(id),
    spec_json TEXT NOT NULL,
    status TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL DEFAULT '[]',
    artifact_ids_json TEXT NOT NULL DEFAULT '[]',
    last_error_code TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_steps_plan_status ON steps(plan_id, status);

CREATE TABLE IF NOT EXISTS step_dependencies (
    step_id TEXT NOT NULL REFERENCES steps(step_id),
    depends_on_step_id TEXT NOT NULL REFERENCES steps(step_id),
    PRIMARY KEY(step_id, depends_on_step_id)
);

CREATE TABLE IF NOT EXISTS events (
    event_id TEXT PRIMARY KEY,
    event_version INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    run_id TEXT,
    sequence INTEGER NOT NULL,
    timestamp TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    correlation_id TEXT,
    causation_id TEXT,
    payload_json TEXT NOT NULL,
    UNIQUE(aggregate_type, aggregate_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_events_aggregate_sequence ON events(aggregate_type, aggregate_id, sequence);

CREATE TABLE IF NOT EXISTS artifacts (
    artifact_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    kind TEXT NOT NULL,
    media_type TEXT NOT NULL,
    storage_uri TEXT NOT NULL UNIQUE,
    content_hash TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    summary TEXT NOT NULL,
    workspace_relative_path TEXT,
    task_id TEXT REFERENCES tasks(id),
    step_id TEXT REFERENCES steps(step_id),
    verified INTEGER NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_artifacts_task ON artifacts(task_id, step_id);

CREATE TABLE IF NOT EXISTS evidence (
    evidence_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    claim TEXT NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(artifact_id),
    location TEXT NOT NULL,
    verification_method TEXT NOT NULL,
    verified_at TEXT NOT NULL,
    confidence REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS checkpoints (
    checkpoint_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    sequence INTEGER NOT NULL,
    snapshot_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(task_id, sequence)
);

CREATE TABLE IF NOT EXISTS tool_calls (
    tool_call_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    run_id TEXT,
    step_id TEXT REFERENCES steps(step_id),
    tool_name TEXT NOT NULL,
    normalized_arguments_hash TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    risk_class TEXT NOT NULL,
    status TEXT NOT NULL,
    result_json TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS approvals (
    approval_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    step_id TEXT REFERENCES steps(step_id),
    tool_name TEXT NOT NULL,
    normalized_arguments_hash TEXT NOT NULL,
    decision TEXT NOT NULL,
    expires_at TEXT,
    uses_remaining INTEGER NOT NULL,
    created_at TEXT NOT NULL
);
