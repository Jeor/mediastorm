-- +goose Up
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS client_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_sessions_client_id ON sessions(client_id) WHERE client_id <> '';

-- +goose Down
DROP INDEX IF EXISTS idx_sessions_client_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS client_id;
