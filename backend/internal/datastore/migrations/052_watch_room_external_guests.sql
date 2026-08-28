-- +goose Up
CREATE TABLE watch_room_external_invites (
    room_id TEXT PRIMARY KEY REFERENCES watch_rooms(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    short_code TEXT NOT NULL UNIQUE,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_watch_room_external_invites_lookup
    ON watch_room_external_invites(token_hash, active, expires_at);

CREATE TABLE watch_room_guest_members (
    room_id TEXT NOT NULL REFERENCES watch_rooms(id) ON DELETE CASCADE,
    guest_id TEXT NOT NULL,
    name TEXT NOT NULL,
    client_id TEXT NOT NULL DEFAULT '',
    ready BOOLEAN NOT NULL DEFAULT false,
    buffering BOOLEAN NOT NULL DEFAULT false,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    capabilities JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (room_id, guest_id)
);

CREATE INDEX idx_watch_room_guest_members_presence
    ON watch_room_guest_members(room_id, last_seen_at);

-- +goose Down
DROP TABLE IF EXISTS watch_room_guest_members;
DROP TABLE IF EXISTS watch_room_external_invites;
