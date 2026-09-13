-- +goose Up
ALTER TABLE sports_teams ADD COLUMN IF NOT EXISTS location TEXT NOT NULL DEFAULT '';
ALTER TABLE sports_teams ADD COLUMN IF NOT EXISTS nickname TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sports_teams DROP COLUMN IF EXISTS nickname;
ALTER TABLE sports_teams DROP COLUMN IF EXISTS location;
