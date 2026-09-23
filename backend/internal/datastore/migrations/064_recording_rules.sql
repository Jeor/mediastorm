-- +goose Up
CREATE TABLE recording_rules (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    match_type TEXT NOT NULL CHECK (match_type IN ('title', 'regex')),
    pattern TEXT NOT NULL,
    channel_id TEXT NOT NULL DEFAULT '',
    tvg_id TEXT NOT NULL DEFAULT '',
    channel_name TEXT NOT NULL DEFAULT '',
    all_channels BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    padding_before_seconds INTEGER NOT NULL DEFAULT 300,
    padding_after_seconds INTEGER NOT NULL DEFAULT 300,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_recording_rules_user_id ON recording_rules(user_id);
CREATE INDEX idx_recording_rules_enabled ON recording_rules(enabled) WHERE enabled = TRUE;

ALTER TABLE recordings
    ADD COLUMN rule_id TEXT REFERENCES recording_rules(id) ON DELETE SET NULL,
    ADD COLUMN schedule_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_recordings_rule_schedule
    ON recordings(rule_id, schedule_key)
    WHERE rule_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_recordings_rule_schedule;
ALTER TABLE recordings DROP COLUMN IF EXISTS schedule_key;
ALTER TABLE recordings DROP COLUMN IF EXISTS rule_id;
DROP TABLE IF EXISTS recording_rules;
