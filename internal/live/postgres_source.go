package live

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresVersionSource struct {
	pool *pgxpool.Pool
}

func NewPostgresVersionSource(pool *pgxpool.Pool) *PostgresVersionSource {
	return &PostgresVersionSource{pool: pool}
}

func (s *PostgresVersionSource) Versions(
	ctx context.Context,
	meetingIDs []uuid.UUID,
) (map[uuid.UUID]int64, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, version FROM meetings WHERE id = ANY($1::uuid[])`,
		meetingIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query meeting versions: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]int64, len(meetingIDs))
	for rows.Next() {
		var meetingID uuid.UUID
		var version int64
		if err := rows.Scan(&meetingID, &version); err != nil {
			return nil, fmt.Errorf("scan meeting version: %w", err)
		}
		result[meetingID] = version
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate meeting versions: %w", err)
	}
	return result, nil
}
