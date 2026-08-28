package datastore

import (
	"context"
	"encoding/json"
	"fmt"

	"novastream/models"
)

type pgRealtimeScrobbleSessionRepo struct {
	pool DB
}

func (r *pgRealtimeScrobbleSessionRepo) Upsert(ctx context.Context, session *models.RealtimeScrobbleSession) error {
	payload, err := json.Marshal(session.Update)
	if err != nil {
		return fmt.Errorf("marshal realtime scrobble session: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO realtime_scrobble_sessions
		(provider, user_id, media_type, item_id, remote_key, state, percent_watched, update_payload, started_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
		ON CONFLICT (provider, user_id, media_type, item_id) DO UPDATE SET
		remote_key=EXCLUDED.remote_key, state=EXCLUDED.state, percent_watched=EXCLUDED.percent_watched,
		update_payload=EXCLUDED.update_payload, updated_at=NOW()`,
		session.Provider, session.UserID, session.MediaType, session.ItemID, session.RemoteKey,
		session.State, session.PercentWatched, payload, session.StartedAt)
	if err != nil {
		return fmt.Errorf("upsert realtime scrobble session: %w", err)
	}
	return nil
}

func (r *pgRealtimeScrobbleSessionRepo) List(ctx context.Context) ([]models.RealtimeScrobbleSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT provider, user_id, media_type, item_id, remote_key, state, percent_watched,
		update_payload, started_at, updated_at
		FROM realtime_scrobble_sessions ORDER BY updated_at`)
	if err != nil {
		return nil, fmt.Errorf("list realtime scrobble sessions: %w", err)
	}
	defer rows.Close()
	var sessions []models.RealtimeScrobbleSession
	for rows.Next() {
		var session models.RealtimeScrobbleSession
		var payload []byte
		if err := rows.Scan(&session.Provider, &session.UserID, &session.MediaType, &session.ItemID,
			&session.RemoteKey, &session.State, &session.PercentWatched, &payload,
			&session.StartedAt, &session.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan realtime scrobble session: %w", err)
		}
		if err := json.Unmarshal(payload, &session.Update); err != nil {
			return nil, fmt.Errorf("decode realtime scrobble session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (r *pgRealtimeScrobbleSessionRepo) Delete(ctx context.Context, provider, userID, mediaType, itemID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM realtime_scrobble_sessions
		WHERE provider=$1 AND user_id=$2 AND media_type=$3 AND item_id=$4`,
		provider, userID, mediaType, itemID)
	return err
}
