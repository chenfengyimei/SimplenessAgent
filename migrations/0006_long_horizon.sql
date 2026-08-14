CREATE TABLE IF NOT EXISTS task_horizons (
    horizon_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL UNIQUE REFERENCES tasks(id),
    version INTEGER NOT NULL,
    state_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_task_horizons_status ON task_horizons(updated_at DESC);

CREATE TABLE IF NOT EXISTS model_role_profiles (
    profile_id TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL REFERENCES deployments(deployment_id),
    role TEXT NOT NULL,
    version INTEGER NOT NULL,
    profile_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(deployment_id, role)
);
