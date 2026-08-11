CREATE TABLE IF NOT EXISTS deployments (
    deployment_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    name TEXT NOT NULL UNIQUE,
    provider_type TEXT NOT NULL,
    location TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    credential_ref TEXT,
    model TEXT NOT NULL,
    runtime TEXT,
    runtime_version TEXT,
    quantization TEXT,
    model_profile_id TEXT,
    capability_snapshot_id TEXT,
    enabled INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS deployment_capabilities (
    capability_snapshot_id TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL REFERENCES deployments(deployment_id),
    version INTEGER NOT NULL,
    snapshot_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_deployment_capabilities_deployment ON deployment_capabilities(deployment_id, created_at DESC);
