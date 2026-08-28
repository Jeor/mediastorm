-- Speed startup/background cleanup of expired persisted prequeue entries.
-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_prequeue_expires_at ON prequeue(expires_at);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_prequeue_expires_at;
