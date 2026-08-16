-- Persist account-level progress for the hidden Numbers Station puzzle.
-- +goose Up
CREATE TABLE IF NOT EXISTS numbers_station_progress (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    stage INTEGER NOT NULL DEFAULT 0 CHECK (stage >= 0),
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS numbers_station_progress;
