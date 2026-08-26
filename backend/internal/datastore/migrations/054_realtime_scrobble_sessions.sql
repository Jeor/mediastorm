-- +goose Up
CREATE TABLE realtime_scrobble_sessions (
    provider TEXT NOT NULL,
    user_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    item_id TEXT NOT NULL,
    remote_key TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL,
    percent_watched DOUBLE PRECISION NOT NULL DEFAULT 0,
    update_payload JSONB NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, user_id, media_type, item_id)
);

CREATE INDEX idx_realtime_scrobble_sessions_updated
    ON realtime_scrobble_sessions (updated_at);

-- +goose Down
DROP TABLE IF EXISTS realtime_scrobble_sessions;
