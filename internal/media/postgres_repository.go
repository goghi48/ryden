package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetUserAvatar(
	ctx context.Context,
	viewerID, userID uuid.UUID,
) (Photo, error) {
	var result Photo
	err := r.pool.QueryRow(ctx, `
		SELECT photo.content_type, photo.content, photo.content_hash, photo.updated_at
		FROM user_avatar_photos photo
		JOIN users viewer ON viewer.id = $1
		WHERE photo.user_id = $2
	`, viewerID, userID).Scan(
		&result.ContentType, &result.Content, &result.ContentHash, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	if err != nil {
		return Photo{}, fmt.Errorf("get user avatar: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) PutUserAvatar(
	ctx context.Context,
	userID uuid.UUID,
	photo Photo,
) (AvatarMutation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AvatarMutation{}, fmt.Errorf("begin avatar update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentRevision pgtype.Int8
	var legacyURL pgtype.Text
	var userUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT avatar_revision, avatar_url, updated_at
		FROM users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&currentRevision, &legacyURL, &userUpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return AvatarMutation{}, ErrNotFound
	} else if err != nil {
		return AvatarMutation{}, fmt.Errorf("lock user for avatar update: %w", err)
	}

	var priorHash []byte
	var photoUpdatedAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT content_hash, updated_at FROM user_avatar_photos WHERE user_id = $1
	`, userID).Scan(&priorHash, &photoUpdatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return AvatarMutation{}, fmt.Errorf("read current avatar hash: %w", err)
	}
	if bytes.Equal(priorHash, photo.ContentHash) && !legacyURL.Valid && currentRevision.Valid {
		if err := tx.Commit(ctx); err != nil {
			return AvatarMutation{}, fmt.Errorf("commit unchanged avatar: %w", err)
		}
		return AvatarMutation{AvatarRevision: int64Pointer(currentRevision.Int64), UpdatedAt: photoUpdatedAt}, nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_avatar_photos (user_id, content_type, content, content_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE
		SET content_type = EXCLUDED.content_type,
		    content = EXCLUDED.content,
		    content_hash = EXCLUDED.content_hash,
		    updated_at = now()
	`, userID, photo.ContentType, photo.Content, photo.ContentHash); err != nil {
		return AvatarMutation{}, fmt.Errorf("store user avatar: %w", err)
	}
	var revision int64
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `
		UPDATE users
		SET avatar_revision = COALESCE(avatar_revision, 0) + 1,
		    avatar_url = NULL,
		    updated_at = now()
		WHERE id = $1
		RETURNING avatar_revision, updated_at
	`, userID).Scan(&revision, &updatedAt); err != nil {
		return AvatarMutation{}, fmt.Errorf("update avatar revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AvatarMutation{}, fmt.Errorf("commit avatar update: %w", err)
	}
	return AvatarMutation{AvatarRevision: int64Pointer(revision), Changed: true, UpdatedAt: updatedAt}, nil
}

func (r *PostgresRepository) DeleteUserAvatar(
	ctx context.Context,
	userID uuid.UUID,
) (AvatarMutation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AvatarMutation{}, fmt.Errorf("begin avatar deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var revision pgtype.Int8
	var legacyURL pgtype.Text
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT avatar_revision, avatar_url, updated_at
		FROM users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&revision, &legacyURL, &updatedAt); errors.Is(err, pgx.ErrNoRows) {
		return AvatarMutation{}, ErrNotFound
	} else if err != nil {
		return AvatarMutation{}, fmt.Errorf("lock user for avatar deletion: %w", err)
	}
	if !revision.Valid && !legacyURL.Valid {
		if err := tx.Commit(ctx); err != nil {
			return AvatarMutation{}, fmt.Errorf("commit absent avatar: %w", err)
		}
		return AvatarMutation{UpdatedAt: updatedAt}, nil
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_avatar_photos WHERE user_id = $1", userID); err != nil {
		return AvatarMutation{}, fmt.Errorf("delete user avatar: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		UPDATE users
		SET avatar_revision = NULL, avatar_url = NULL, updated_at = now()
		WHERE id = $1
		RETURNING updated_at
	`, userID).Scan(&updatedAt); err != nil {
		return AvatarMutation{}, fmt.Errorf("clear avatar revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AvatarMutation{}, fmt.Errorf("commit avatar deletion: %w", err)
	}
	return AvatarMutation{Changed: true, UpdatedAt: updatedAt}, nil
}

func int64Pointer(value int64) *int64 {
	return &value
}

func (r *PostgresRepository) GetMeetingPhoto(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) (Photo, error) {
	var result Photo
	err := r.pool.QueryRow(ctx, `
		SELECT photo.content_type, photo.content, photo.content_hash, photo.updated_at
		FROM meeting_photos photo
		JOIN meeting_participants participant
		  ON participant.meeting_id = photo.meeting_id
		 AND participant.user_id = $2
		 AND participant.status = 'active'
		WHERE photo.meeting_id = $1
	`, meetingID, userID).Scan(
		&result.ContentType, &result.Content, &result.ContentHash, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	if err != nil {
		return Photo{}, fmt.Errorf("get meeting photo: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) PutMeetingPhoto(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	expectedVersion int64,
	photo Photo,
) (Mutation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Mutation{}, fmt.Errorf("begin meeting photo update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := lockMeetingOwner(ctx, tx, ownerID, meetingID)
	if err != nil {
		return Mutation{}, err
	}
	if !locked.meetingPhotoEditable() {
		return Mutation{}, ErrNotEditable
	}

	var priorHash []byte
	err = tx.QueryRow(ctx,
		"SELECT content_hash FROM meeting_photos WHERE meeting_id = $1",
		meetingID,
	).Scan(&priorHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Mutation{}, fmt.Errorf("read current meeting photo hash: %w", err)
	}
	if bytes.Equal(priorHash, photo.ContentHash) && !locked.CoverURL.Valid {
		updatedAt, err := meetingUpdatedAt(ctx, tx, meetingID)
		if err != nil {
			return Mutation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Mutation{}, fmt.Errorf("commit unchanged meeting photo: %w", err)
		}
		return Mutation{Version: locked.Version, UpdatedAt: updatedAt}, nil
	}
	if locked.Version != expectedVersion {
		return Mutation{}, ErrVersionConflict
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO meeting_photos (meeting_id, content_type, content, content_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (meeting_id) DO UPDATE
		SET content_type = EXCLUDED.content_type,
		    content = EXCLUDED.content,
		    content_hash = EXCLUDED.content_hash,
		    updated_at = now()
	`, meetingID, photo.ContentType, photo.Content, photo.ContentHash); err != nil {
		return Mutation{}, fmt.Errorf("store meeting photo: %w", err)
	}
	mutation, err := touchMeeting(ctx, tx, meetingID, true)
	if err != nil {
		return Mutation{}, err
	}
	if _, err := tx.Exec(ctx, "UPDATE meetings SET cover_url = NULL WHERE id = $1", meetingID); err != nil {
		return Mutation{}, fmt.Errorf("clear legacy meeting cover: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Mutation{}, fmt.Errorf("commit meeting photo update: %w", err)
	}
	return mutation, nil
}

func (r *PostgresRepository) DeleteMeetingPhoto(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	expectedVersion int64,
) (Mutation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Mutation{}, fmt.Errorf("begin meeting photo deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := lockMeetingOwner(ctx, tx, ownerID, meetingID)
	if err != nil {
		return Mutation{}, err
	}
	if !locked.meetingPhotoEditable() {
		return Mutation{}, ErrNotEditable
	}
	var photoExists bool
	if err := tx.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM meeting_photos WHERE meeting_id = $1)",
		meetingID,
	).Scan(&photoExists); err != nil {
		return Mutation{}, fmt.Errorf("check current meeting photo: %w", err)
	}
	if !photoExists && !locked.CoverURL.Valid {
		updatedAt, err := meetingUpdatedAt(ctx, tx, meetingID)
		if err != nil {
			return Mutation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Mutation{}, fmt.Errorf("commit absent meeting photo: %w", err)
		}
		return Mutation{Version: locked.Version, UpdatedAt: updatedAt}, nil
	}
	if locked.Version != expectedVersion {
		return Mutation{}, ErrVersionConflict
	}
	if _, err := tx.Exec(ctx, "DELETE FROM meeting_photos WHERE meeting_id = $1", meetingID); err != nil {
		return Mutation{}, fmt.Errorf("delete meeting photo: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE meetings SET cover_url = NULL WHERE id = $1", meetingID); err != nil {
		return Mutation{}, fmt.Errorf("clear legacy meeting cover: %w", err)
	}
	mutation, err := touchMeeting(ctx, tx, meetingID, true)
	if err != nil {
		return Mutation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Mutation{}, fmt.Errorf("commit meeting photo deletion: %w", err)
	}
	return mutation, nil
}

func (r *PostgresRepository) GetPlanOptionPhoto(
	ctx context.Context,
	userID, meetingID, optionID uuid.UUID,
) (Photo, error) {
	var result Photo
	err := r.pool.QueryRow(ctx, `
		SELECT photo.content_type, photo.content, photo.content_hash, photo.updated_at
		FROM plan_option_photos photo
		JOIN plan_options option ON option.id = photo.plan_option_id
		JOIN meeting_participants participant
		  ON participant.meeting_id = option.meeting_id
		 AND participant.user_id = $3
		 AND participant.status = 'active'
		WHERE photo.plan_option_id = $2 AND option.meeting_id = $1
	`, meetingID, optionID, userID).Scan(
		&result.ContentType, &result.Content, &result.ContentHash, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	if err != nil {
		return Photo{}, fmt.Errorf("get plan option photo: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) PutPlanOptionPhoto(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	expectedVersion int64,
	photo Photo,
) (Mutation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Mutation{}, fmt.Errorf("begin plan photo update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := lockMeetingOwner(ctx, tx, ownerID, meetingID)
	if err != nil {
		return Mutation{}, err
	}
	if locked.State != "draft" {
		return Mutation{}, ErrNotEditable
	}
	if err := ensurePlanOption(ctx, tx, meetingID, optionID); err != nil {
		return Mutation{}, err
	}

	var priorHash []byte
	err = tx.QueryRow(ctx,
		"SELECT content_hash FROM plan_option_photos WHERE plan_option_id = $1",
		optionID,
	).Scan(&priorHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Mutation{}, fmt.Errorf("read current plan photo hash: %w", err)
	}
	if bytes.Equal(priorHash, photo.ContentHash) {
		_, updatedAt, err := meetingVersionAndUpdatedAt(ctx, tx, meetingID)
		if err != nil {
			return Mutation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Mutation{}, fmt.Errorf("commit unchanged plan photo: %w", err)
		}
		return Mutation{Version: locked.Version, UpdatedAt: updatedAt}, nil
	}
	if locked.Version != expectedVersion {
		return Mutation{}, ErrVersionConflict
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO plan_option_photos (plan_option_id, content_type, content, content_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (plan_option_id) DO UPDATE
		SET content_type = EXCLUDED.content_type,
		    content = EXCLUDED.content,
		    content_hash = EXCLUDED.content_hash,
		    updated_at = now()
	`, optionID, photo.ContentType, photo.Content, photo.ContentHash); err != nil {
		return Mutation{}, fmt.Errorf("store plan photo: %w", err)
	}
	mutation, err := touchMeeting(ctx, tx, meetingID, true)
	if err != nil {
		return Mutation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Mutation{}, fmt.Errorf("commit plan photo update: %w", err)
	}
	return mutation, nil
}

func (r *PostgresRepository) DeletePlanOptionPhoto(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	expectedVersion int64,
) (Mutation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Mutation{}, fmt.Errorf("begin plan photo deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := lockMeetingOwner(ctx, tx, ownerID, meetingID)
	if err != nil {
		return Mutation{}, err
	}
	if locked.State != "draft" {
		return Mutation{}, ErrNotEditable
	}
	if err := ensurePlanOption(ctx, tx, meetingID, optionID); err != nil {
		return Mutation{}, err
	}
	var photoExists bool
	if err := tx.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM plan_option_photos WHERE plan_option_id = $1)",
		optionID,
	).Scan(&photoExists); err != nil {
		return Mutation{}, fmt.Errorf("check current plan photo: %w", err)
	}
	if !photoExists {
		_, updatedAt, err := meetingVersionAndUpdatedAt(ctx, tx, meetingID)
		if err != nil {
			return Mutation{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Mutation{}, fmt.Errorf("commit absent plan photo: %w", err)
		}
		return Mutation{Version: locked.Version, UpdatedAt: updatedAt}, nil
	}
	if locked.Version != expectedVersion {
		return Mutation{}, ErrVersionConflict
	}
	if _, err := tx.Exec(ctx,
		"DELETE FROM plan_option_photos WHERE plan_option_id = $1",
		optionID,
	); err != nil {
		return Mutation{}, fmt.Errorf("delete plan photo: %w", err)
	}
	mutation, err := touchMeeting(ctx, tx, meetingID, true)
	if err != nil {
		return Mutation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Mutation{}, fmt.Errorf("commit plan photo deletion: %w", err)
	}
	return mutation, nil
}

type lockedMeeting struct {
	State            string
	CoordinationMode string
	Version          int64
	CoverURL         pgtype.Text
	Archived         bool
}

func (meeting lockedMeeting) meetingPhotoEditable() bool {
	return meeting.State == "draft" ||
		(meeting.State == "scheduled" && meeting.CoordinationMode == "fixed" && !meeting.Archived)
}

func lockMeetingOwner(
	ctx context.Context,
	tx pgx.Tx,
	ownerID, meetingID uuid.UUID,
) (lockedMeeting, error) {
	var result lockedMeeting
	err := tx.QueryRow(ctx, `
		SELECT m.state, m.coordination_mode, m.version, m.cover_url,
		       CASE
		           WHEN m.state = 'scheduled' AND selected_time.id IS NOT NULL
		               THEN COALESCE(selected_time.ends_at, selected_time.starts_at) + interval '24 hours' <= now()
		           ELSE false
		       END AS archived
		FROM meetings m
		LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
		WHERE m.id = $1 AND m.owner_id = $2
		FOR UPDATE OF m
	`, meetingID, ownerID).Scan(
		&result.State, &result.CoordinationMode, &result.Version, &result.CoverURL, &result.Archived,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedMeeting{}, ErrNotFound
	}
	if err != nil {
		return lockedMeeting{}, fmt.Errorf("lock meeting for photo update: %w", err)
	}
	return result, nil
}

func ensurePlanOption(
	ctx context.Context,
	tx pgx.Tx,
	meetingID, optionID uuid.UUID,
) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM plan_options WHERE id = $1 AND meeting_id = $2
		)
	`, optionID, meetingID).Scan(&exists); err != nil {
		return fmt.Errorf("check plan option for photo: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func touchMeeting(
	ctx context.Context,
	tx pgx.Tx,
	meetingID uuid.UUID,
	changed bool,
) (Mutation, error) {
	var result Mutation
	err := tx.QueryRow(ctx, `
		UPDATE meetings
		SET version = version + 1, updated_at = now()
		WHERE id = $1
		RETURNING version, updated_at
	`, meetingID).Scan(&result.Version, &result.UpdatedAt)
	if err != nil {
		return Mutation{}, fmt.Errorf("touch meeting after photo change: %w", err)
	}
	result.Changed = changed
	return result, nil
}

func meetingVersionAndUpdatedAt(
	ctx context.Context,
	tx pgx.Tx,
	meetingID uuid.UUID,
) (int64, time.Time, error) {
	var version int64
	var updatedAt time.Time
	if err := tx.QueryRow(ctx,
		"SELECT version, updated_at FROM meetings WHERE id = $1",
		meetingID,
	).Scan(&version, &updatedAt); err != nil {
		return 0, time.Time{}, fmt.Errorf("read meeting photo version: %w", err)
	}
	return version, updatedAt, nil
}

func meetingUpdatedAt(
	ctx context.Context,
	tx pgx.Tx,
	meetingID uuid.UUID,
) (time.Time, error) {
	_, updatedAt, err := meetingVersionAndUpdatedAt(ctx, tx, meetingID)
	return updatedAt, err
}
