-- Add scheduled_at column to recurrence_rules table
ALTER TABLE recurrence_rules 
ADD COLUMN scheduled_at TIMESTAMP;

-- Create index on scheduled_at for better query performance
CREATE INDEX idx_recurrence_rules_scheduled_at ON recurrence_rules(scheduled_at);
