package lookuppg

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/lookupstore"
)

type Store struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

func NewStore(db *pgxpool.Pool, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{db: db, log: log.With("component", "lookuppg")}
}

var _ lookupstore.LookupReader = (*Store)(nil)

func (s *Store) HeroIDs(ctx context.Context) (map[int]struct{}, error) {
	rows, err := s.db.Query(ctx, "SELECT id FROM heroes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[int]struct{})
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m[id] = struct{}{}
	}
	return m, rows.Err()
}

func (s *Store) ItemIDs(ctx context.Context) (map[int]struct{}, error) {
	rows, err := s.db.Query(ctx, "SELECT id FROM items")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[int]struct{})
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m[id] = struct{}{}
	}
	return m, rows.Err()
}

func (s *Store) PatchByTimestamp(ctx context.Context, unixSec int64) (lookupstore.PatchInfo, error) {
	var p lookupstore.PatchInfo
	err := s.db.QueryRow(ctx, `
		SELECT id, name, release_at
		FROM patches
		WHERE extract(epoch from release_at) <= $1
		ORDER BY release_at DESC LIMIT 1
	`, unixSec).Scan(&p.ID, &p.Name, &p.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	return p, err
}