ALTER TABLE agent
ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'normal'
CHECK (execution_mode IN ('normal', 'goal'));
