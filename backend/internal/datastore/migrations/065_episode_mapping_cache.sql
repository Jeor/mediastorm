-- +goose Up
CREATE TABLE episode_mapping_cache (
    source_key TEXT PRIMARY KEY,
    body BYTEA NOT NULL,
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE episode_mapping_cache;
