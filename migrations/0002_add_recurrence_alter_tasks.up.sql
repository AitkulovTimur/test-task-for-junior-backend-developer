CREATE TABLE IF NOT EXISTS recurrence_rules (
    id BIGSERIAL PRIMARY KEY,
    type TEXT NOT NULL,
    params JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS parent_rule_id BIGINT REFERENCES recurrence_rules(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS is_modified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_tasks_due_date ON tasks (scheduled_at);
CREATE INDEX IF NOT EXISTS idx_tasks_parent_rule ON tasks (parent_rule_id);