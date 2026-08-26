-- +goose Up
CREATE TABLE watch_room_external_sources (
    room_id TEXT PRIMARY KEY REFERENCES watch_rooms(id) ON DELETE CASCADE,
    resource TEXT NOT NULL,
    params JSONB NOT NULL DEFAULT '{}',
    bound_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS watch_room_external_sources;
