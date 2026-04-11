ALTER TABLE recurrence_rules
ADD COLUMN scheduled_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_recurrence_rules_type ON recurrence_rules(type);
