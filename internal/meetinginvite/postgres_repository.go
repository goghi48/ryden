package meetinginvite

import (
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

func (r *PostgresRepository) Candidates(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	limit, offset int,
) (CandidatePage, error) {
	if err := r.authorizeActiveOwner(ctx, r.pool, ownerID, meetingID, false); err != nil {
		return CandidatePage{}, err
	}
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM friendships f
		WHERE f.status = 'accepted'
		  AND (f.requester_id = $1 OR f.addressee_id = $1)
	`, ownerID).Scan(&total); err != nil {
		return CandidatePage{}, fmt.Errorf("count friend invitation candidates: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.nickname, u.display_name, u.avatar_url, u.avatar_revision,
		       fi.id, fi.status,
		       EXISTS (
		           SELECT 1 FROM meeting_participants mp
		           WHERE mp.meeting_id = $2 AND mp.user_id = u.id AND mp.status = 'active'
		       ) AS is_participant
		FROM friendships f
		JOIN users u ON u.id = CASE WHEN f.requester_id = $1 THEN f.addressee_id ELSE f.requester_id END
		LEFT JOIN meeting_friend_invitations fi ON fi.meeting_id = $2 AND fi.invitee_id = u.id
		WHERE f.status = 'accepted'
		  AND (f.requester_id = $1 OR f.addressee_id = $1)
		ORDER BY u.display_name, u.nickname, u.id
		LIMIT $3 OFFSET $4
	`, ownerID, meetingID, limit, offset)
	if err != nil {
		return CandidatePage{}, fmt.Errorf("list friend invitation candidates: %w", err)
	}
	defer rows.Close()
	items := make([]Candidate, 0, limit)
	for rows.Next() {
		var item Candidate
		var avatarURL pgtype.Text
		var avatarRevision pgtype.Int8
		var invitationID pgtype.UUID
		var invitationStatus pgtype.Text
		if err := rows.Scan(
			&item.UserID, &item.Nickname, &item.DisplayName, &avatarURL, &avatarRevision,
			&invitationID, &invitationStatus, &item.IsParticipant,
		); err != nil {
			return CandidatePage{}, fmt.Errorf("scan friend invitation candidate: %w", err)
		}
		item.AvatarURL = textPointer(avatarURL)
		item.AvatarRevision = int8Pointer(avatarRevision)
		item.InvitationID = uuidPointer(invitationID)
		item.InvitationStatus = textPointer(invitationStatus)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CandidatePage{}, fmt.Errorf("iterate friend invitation candidates: %w", err)
	}
	return CandidatePage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (r *PostgresRepository) Incoming(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int,
) (IncomingPage, error) {
	const activeFilter = `
		fi.invitee_id = $1
		AND fi.status = 'pending'
		AND m.state IN ('draft', 'collecting', 'scheduled')
		AND (
			COALESCE(selected_time.ends_at, selected_time.starts_at) IS NULL
			OR COALESCE(selected_time.ends_at, selected_time.starts_at) + interval '24 hours' > now()
		)
		AND NOT EXISTS (
			SELECT 1 FROM meeting_participants mp
			WHERE mp.meeting_id = m.id AND mp.user_id = $1 AND mp.status = 'active'
		)`
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM meeting_friend_invitations fi
		JOIN meetings m ON m.id = fi.meeting_id
		LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
		WHERE `+activeFilter, userID).Scan(&total); err != nil {
		return IncomingPage{}, fmt.Errorf("count incoming meeting invitations: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT fi.id, m.id, m.title, owner.display_name,
		       selected_time.starts_at, selected_time.ends_at, m.timezone, fi.created_at
		FROM meeting_friend_invitations fi
		JOIN meetings m ON m.id = fi.meeting_id
		JOIN users owner ON owner.id = m.owner_id
		LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
		WHERE `+activeFilter+`
		ORDER BY fi.created_at DESC, fi.id DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return IncomingPage{}, fmt.Errorf("list incoming meeting invitations: %w", err)
	}
	defer rows.Close()
	items := make([]Incoming, 0, limit)
	for rows.Next() {
		var item Incoming
		var startsAt, endsAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.ID, &item.MeetingID, &item.MeetingTitle, &item.OwnerDisplayName,
			&startsAt, &endsAt, &item.Timezone, &item.CreatedAt,
		); err != nil {
			return IncomingPage{}, fmt.Errorf("scan incoming meeting invitation: %w", err)
		}
		item.StartsAt = timePointer(startsAt)
		item.EndsAt = timePointer(endsAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return IncomingPage{}, fmt.Errorf("iterate incoming meeting invitations: %w", err)
	}
	return IncomingPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (r *PostgresRepository) Send(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	userIDs []uuid.UUID,
) (int, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin direct meeting invitations: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.authorizeActiveOwner(ctx, tx, ownerID, meetingID, true); err != nil {
		return 0, err
	}
	changed := 0
	for _, userID := range userIDs {
		result, err := tx.Exec(ctx, `
			INSERT INTO meeting_friend_invitations (id, meeting_id, invited_by, invitee_id)
			SELECT $1, $2, $3, $4
			WHERE EXISTS (
				SELECT 1 FROM friendships f
				WHERE f.status = 'accepted'
				  AND ((f.requester_id = $3 AND f.addressee_id = $4)
				    OR (f.requester_id = $4 AND f.addressee_id = $3))
			)
			AND NOT EXISTS (
				SELECT 1 FROM meeting_participants mp
				WHERE mp.meeting_id = $2 AND mp.user_id = $4 AND mp.status = 'active'
			)
			ON CONFLICT (meeting_id, invitee_id) DO NOTHING
		`, uuid.New(), meetingID, ownerID, userID)
		if err != nil {
			return 0, fmt.Errorf("insert direct meeting invitation: %w", err)
		}
		changed += int(result.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit direct meeting invitations: %w", err)
	}
	return changed, nil
}

func (r *PostgresRepository) Accept(
	ctx context.Context,
	userID, invitationID uuid.UUID,
) (ResponseMutation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ResponseMutation{}, fmt.Errorf("begin accepting meeting invitation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	meetingID, status, state, selectedEnd, err := lockInvitation(ctx, tx, userID, invitationID)
	if err != nil {
		return ResponseMutation{}, err
	}
	if status == "accepted" {
		if err := tx.Commit(ctx); err != nil {
			return ResponseMutation{}, fmt.Errorf("commit repeated meeting invitation acceptance: %w", err)
		}
		return ResponseMutation{MeetingID: meetingID}, nil
	}
	if status == "declined" {
		return ResponseMutation{}, ErrConflict
	}
	if !meetingAcceptsParticipants(state, selectedEnd) {
		return ResponseMutation{}, ErrConflict
	}
	joinResult, err := tx.Exec(ctx, `
		INSERT INTO meeting_participants (meeting_id, user_id, role, status)
		VALUES ($1, $2, 'participant', 'active')
		ON CONFLICT (meeting_id, user_id) DO UPDATE
		SET role = 'participant', status = 'active', joined_at = now()
		WHERE meeting_participants.status <> 'active'
	`, meetingID, userID)
	if err != nil {
		return ResponseMutation{}, fmt.Errorf("join accepted direct meeting invitation: %w", err)
	}
	joined := joinResult.RowsAffected() > 0
	if _, err := tx.Exec(ctx, `
		UPDATE meeting_friend_invitations
		SET status = 'accepted', responded_at = now()
		WHERE id = $1
	`, invitationID); err != nil {
		return ResponseMutation{}, fmt.Errorf("mark direct meeting invitation accepted: %w", err)
	}
	if joined {
		if _, err := tx.Exec(ctx, `
			UPDATE meetings SET version = version + 1, updated_at = now() WHERE id = $1
		`, meetingID); err != nil {
			return ResponseMutation{}, fmt.Errorf("touch meeting after direct invitation acceptance: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ResponseMutation{}, fmt.Errorf("commit accepting meeting invitation: %w", err)
	}
	return ResponseMutation{MeetingID: meetingID, Changed: true, Joined: joined}, nil
}

func (r *PostgresRepository) Decline(
	ctx context.Context,
	userID, invitationID uuid.UUID,
) (ResponseMutation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ResponseMutation{}, fmt.Errorf("begin declining meeting invitation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	meetingID, status, _, _, err := lockInvitation(ctx, tx, userID, invitationID)
	if err != nil {
		return ResponseMutation{}, err
	}
	if status == "accepted" {
		return ResponseMutation{}, ErrConflict
	}
	if status == "declined" {
		if err := tx.Commit(ctx); err != nil {
			return ResponseMutation{}, fmt.Errorf("commit repeated meeting invitation decline: %w", err)
		}
		return ResponseMutation{MeetingID: meetingID}, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE meeting_friend_invitations
		SET status = 'declined', responded_at = now()
		WHERE id = $1
	`, invitationID); err != nil {
		return ResponseMutation{}, fmt.Errorf("mark direct meeting invitation declined: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ResponseMutation{}, fmt.Errorf("commit declining meeting invitation: %w", err)
	}
	return ResponseMutation{MeetingID: meetingID, Changed: true}, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *PostgresRepository) authorizeActiveOwner(
	ctx context.Context,
	db queryer,
	ownerID, meetingID uuid.UUID,
	lock bool,
) error {
	lockClause := ""
	if lock {
		lockClause = " FOR UPDATE OF m"
	}
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT m.owner_id = $1
		       AND m.state IN ('draft', 'collecting', 'scheduled')
		       AND (
		           COALESCE(selected_time.ends_at, selected_time.starts_at) IS NULL
		           OR COALESCE(selected_time.ends_at, selected_time.starts_at) + interval '24 hours' > now()
		       )
		FROM meetings m
		LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
		WHERE m.id = $2`+lockClause,
		ownerID, meetingID,
	).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !allowed {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("authorize direct meeting invitations: %w", err)
	}
	return nil
}

func lockInvitation(
	ctx context.Context,
	tx pgx.Tx,
	userID, invitationID uuid.UUID,
) (uuid.UUID, string, string, *time.Time, error) {
	var meetingID uuid.UUID
	var status, state string
	var selectedEnd pgtype.Timestamptz
	err := tx.QueryRow(ctx, `
		SELECT fi.meeting_id, fi.status, m.state,
		       COALESCE(selected_time.ends_at, selected_time.starts_at)
		FROM meeting_friend_invitations fi
		JOIN meetings m ON m.id = fi.meeting_id
		LEFT JOIN time_options selected_time ON selected_time.id = m.selected_time_option_id
		WHERE fi.id = $1 AND fi.invitee_id = $2
		FOR UPDATE OF fi, m
	`, invitationID, userID).Scan(&meetingID, &status, &state, &selectedEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", "", nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", "", nil, fmt.Errorf("lock direct meeting invitation: %w", err)
	}
	return meetingID, status, state, timePointer(selectedEnd), nil
}

func meetingAcceptsParticipants(state string, selectedEnd *time.Time) bool {
	if state != "draft" && state != "collecting" && state != "scheduled" {
		return false
	}
	return selectedEnd == nil || selectedEnd.Add(24*time.Hour).After(time.Now())
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func int8Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func uuidPointer(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	result := uuid.UUID(value.Bytes)
	return &result
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
