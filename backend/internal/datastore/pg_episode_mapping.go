package datastore

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"novastream/internal/mediaidentity"
)

type pgEpisodeMappingRepo struct{ pool DB }

func (ds *DataStore) EpisodeMappings() mediaidentity.MappingRepository {
	return &pgEpisodeMappingRepo{pool: ds.pool}
}
func (r *pgEpisodeMappingRepo) Get(ctx context.Context, key string) (*mediaidentity.MappingSnapshot, error) {
	s := &mediaidentity.MappingSnapshot{Key: key}
	err := r.pool.QueryRow(ctx, `SELECT body, etag, last_modified, checked_at FROM episode_mapping_cache WHERE source_key=$1`, key).Scan(&s.Body, &s.ETag, &s.Modified, &s.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}
func (r *pgEpisodeMappingRepo) Put(ctx context.Context, s mediaidentity.MappingSnapshot) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO episode_mapping_cache (source_key,body,etag,last_modified,checked_at) VALUES ($1,$2,$3,$4,$5)
 ON CONFLICT (source_key) DO UPDATE SET body=EXCLUDED.body, etag=EXCLUDED.etag, last_modified=EXCLUDED.last_modified, checked_at=EXCLUDED.checked_at`, s.Key, s.Body, s.ETag, s.Modified, s.CheckedAt)
	return err
}
