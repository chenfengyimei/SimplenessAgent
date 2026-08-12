ALTER TABLE tasks ADD COLUMN conversation_id TEXT REFERENCES tasks(id);
CREATE INDEX IF NOT EXISTS idx_tasks_conversation_updated ON tasks(conversation_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS conversation_messages (
    message_id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES tasks(id),
    turn_task_id TEXT REFERENCES tasks(id),
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_conversation_messages_order ON conversation_messages(conversation_id, created_at);
