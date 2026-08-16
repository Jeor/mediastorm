package datastore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"novastream/models"
)

type pgNumbersStationRepo struct {
	pool DB
}

func (r *pgNumbersStationRepo) Get(ctx context.Context, accountID string) (*models.NumbersStationProgress, error) {
	var progress models.NumbersStationProgress
	err := r.pool.QueryRow(ctx, `
		SELECT account_id, stage, completed, started_at, updated_at, completed_at
		FROM numbers_station_progress WHERE account_id = $1`, accountID).Scan(
		&progress.AccountID, &progress.Stage, &progress.Completed,
		&progress.StartedAt, &progress.UpdatedAt, &progress.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get numbers station progress: %w", err)
	}
	return &progress, nil
}

func (r *pgNumbersStationRepo) List(ctx context.Context) ([]models.NumbersStationProgress, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT account_id, stage, completed, started_at, updated_at, completed_at
		FROM numbers_station_progress ORDER BY account_id`)
	if err != nil {
		return nil, fmt.Errorf("list numbers station progress: %w", err)
	}
	defer rows.Close()
	progress := make([]models.NumbersStationProgress, 0)
	for rows.Next() {
		var item models.NumbersStationProgress
		if err := rows.Scan(&item.AccountID, &item.Stage, &item.Completed, &item.StartedAt, &item.UpdatedAt, &item.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan numbers station progress: %w", err)
		}
		progress = append(progress, item)
	}
	return progress, rows.Err()
}

func (r *pgNumbersStationRepo) Upsert(ctx context.Context, progress *models.NumbersStationProgress) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO numbers_station_progress (account_id, stage, completed, started_at, updated_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (account_id) DO UPDATE SET
			stage = EXCLUDED.stage, completed = EXCLUDED.completed,
			started_at = EXCLUDED.started_at, updated_at = EXCLUDED.updated_at,
			completed_at = EXCLUDED.completed_at`,
		progress.AccountID, progress.Stage, progress.Completed, progress.StartedAt, progress.UpdatedAt, progress.CompletedAt)
	if err != nil {
		return fmt.Errorf("upsert numbers station progress: %w", err)
	}
	return nil
}

func (r *pgNumbersStationRepo) Advance(ctx context.Context, accountID string, expectedStage, nextStage int, completed bool, now time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO numbers_station_progress (account_id, stage, completed, started_at, updated_at, completed_at)
		VALUES ($1, $3, $4, $5, $5, CASE WHEN $4 THEN $5 ELSE NULL END)
		ON CONFLICT (account_id) DO UPDATE SET
			stage = EXCLUDED.stage,
			completed = EXCLUDED.completed,
			updated_at = EXCLUDED.updated_at,
			completed_at = CASE WHEN EXCLUDED.completed THEN EXCLUDED.updated_at ELSE numbers_station_progress.completed_at END
		WHERE numbers_station_progress.stage = $2 AND numbers_station_progress.completed = FALSE`,
		accountID, expectedStage, nextStage, completed, now)
	if err != nil {
		return false, fmt.Errorf("advance numbers station progress: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
