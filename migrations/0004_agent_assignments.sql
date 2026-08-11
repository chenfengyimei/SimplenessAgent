CREATE TABLE IF NOT EXISTS agent_assignments (
    agent_id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    task_id TEXT NOT NULL REFERENCES tasks(id),
    step_id TEXT NOT NULL REFERENCES steps(step_id),
    deployment_id TEXT NOT NULL REFERENCES deployments(deployment_id),
    role TEXT NOT NULL,
    depth INTEGER NOT NULL CHECK(depth = 1),
    allowed_tools_json TEXT NOT NULL,
    workspace_scopes_json TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_assignments_active_step
ON agent_assignments(step_id) WHERE status IN ('PENDING', 'RUNNING');
CREATE INDEX IF NOT EXISTS idx_agent_assignments_task ON agent_assignments(task_id, created_at);
