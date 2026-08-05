package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/goghi48/ryden/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateUserWithSession(
	ctx context.Context,
	newUser NewUser,
	session NewRefreshSession,
) (User, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin registration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := database.New(tx)
	row, err := q.CreateUser(ctx, database.CreateUserParams{
		ID:           newUser.ID,
		Email:        newUser.Email,
		PasswordHash: newUser.PasswordHash,
		DisplayName:  newUser.DisplayName,
		Nickname:     newUser.Nickname,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "users_nickname_key" {
				return User{}, ErrNicknameTaken
			}
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	if err := q.CreateRefreshSession(ctx, database.CreateRefreshSessionParams{
		ID:        session.ID,
		UserID:    row.ID,
		TokenHash: session.TokenHash,
		ExpiresAt: session.ExpiresAt,
	}); err != nil {
		return User{}, fmt.Errorf("create registration session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit registration: %w", err)
	}
	return mapUser(row), nil
}

func (r *PostgresRepository) UserByEmail(ctx context.Context, email string) (UserWithPassword, error) {
	row, err := database.New(r.pool).GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserWithPassword{}, ErrInvalidLogin
	}
	if err != nil {
		return UserWithPassword{}, fmt.Errorf("get user by email: %w", err)
	}
	return UserWithPassword{User: mapUser(row), PasswordHash: row.PasswordHash}, nil
}

func (r *PostgresRepository) UserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	row, err := database.New(r.pool).GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return mapUser(row), nil
}

func (r *PostgresRepository) UpdateProfile(
	ctx context.Context,
	userID uuid.UUID,
	displayName string,
	nickname string,
	avatarURL *string,
) (User, error) {
	avatar := pgtype.Text{}
	if avatarURL != nil {
		avatar = pgtype.Text{String: *avatarURL, Valid: true}
	}
	row, err := database.New(r.pool).UpdateUserProfile(ctx, database.UpdateUserProfileParams{
		ID:          userID,
		DisplayName: displayName,
		Nickname:    nickname,
		AvatarUrl:   avatar,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_nickname_key" {
			return User{}, ErrNicknameTaken
		}
		return User{}, fmt.Errorf("update user profile: %w", err)
	}
	return mapUser(row), nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, session NewRefreshSession) error {
	if err := database.New(r.pool).CreateRefreshSession(ctx, database.CreateRefreshSessionParams{
		ID:        session.ID,
		UserID:    session.UserID,
		TokenHash: session.TokenHash,
		ExpiresAt: session.ExpiresAt,
	}); err != nil {
		return fmt.Errorf("create refresh session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RotateSession(
	ctx context.Context,
	currentHash []byte,
	next NewRefreshSession,
) (User, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)

	current, err := q.GetRefreshSessionForUpdate(ctx, currentHash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && current.RevokedAt != nil {
		return User{}, ErrSessionNotActive
	}
	if err != nil {
		return User{}, fmt.Errorf("get refresh session: %w", err)
	}
	next.UserID = current.UserID
	if err := q.CreateRefreshSession(ctx, database.CreateRefreshSessionParams{
		ID:        next.ID,
		UserID:    next.UserID,
		TokenHash: next.TokenHash,
		ExpiresAt: next.ExpiresAt,
	}); err != nil {
		return User{}, fmt.Errorf("create rotated refresh session: %w", err)
	}
	affected, err := q.RotateRefreshSession(ctx, database.RotateRefreshSessionParams{
		ID:         current.ID,
		ReplacedBy: &next.ID,
	})
	if err != nil {
		return User{}, fmt.Errorf("revoke prior refresh session: %w", err)
	}
	if affected != 1 {
		return User{}, ErrSessionNotActive
	}
	row, err := q.GetUserByID(ctx, current.UserID)
	if err != nil {
		return User{}, fmt.Errorf("get refresh user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit refresh rotation: %w", err)
	}
	return mapUser(row), nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, tokenHash []byte) error {
	if _, err := database.New(r.pool).RevokeRefreshSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}

func mapUser(row database.User) User {
	var avatarURL *string
	if row.AvatarUrl.Valid {
		avatarURL = &row.AvatarUrl.String
	}
	return User{
		ID:             row.ID,
		Email:          row.Email,
		DisplayName:    row.DisplayName,
		Nickname:       row.Nickname,
		AvatarURL:      avatarURL,
		AvatarRevision: int8Pointer(row.AvatarRevision),
	}
}

func int8Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
