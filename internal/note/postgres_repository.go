package note

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ryden-app/ryden/internal/database"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	limit, offset int,
) (Page, error) {
	q := database.New(r.pool)
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	} else if err != nil {
		return Page{}, fmt.Errorf("authorize meeting notes: %w", err)
	}
	total, err := q.CountMeetingNotes(ctx, meetingID)
	if err != nil {
		return Page{}, fmt.Errorf("count meeting notes: %w", err)
	}
	rows, err := q.ListMeetingNotes(ctx, database.ListMeetingNotesParams{
		MeetingID: meetingID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return Page{}, fmt.Errorf("list meeting notes: %w", err)
	}
	result := Page{Items: make([]Note, 0, len(rows)), Total: int(total), Limit: limit, Offset: offset}
	for _, row := range rows {
		result.Items = append(result.Items, Note{
			UserID: row.UserID, DisplayName: row.DisplayName, Text: row.Text,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return result, nil
}

func (r *PostgresRepository) Upsert(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	text string,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin meeting note upsert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := authorizeMutation(ctx, q, userID, meetingID); err != nil {
		return false, err
	}
	_, err = q.UpsertMeetingNote(ctx, database.UpsertMeetingNoteParams{
		MeetingID: meetingID, UserID: userID, Text: text,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit unchanged meeting note: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("upsert meeting note: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch meeting after note upsert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit meeting note upsert: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) Delete(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin meeting note deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if err := authorizeMutation(ctx, q, userID, meetingID); err != nil {
		return false, err
	}
	changed, err := q.DeleteMeetingNote(ctx, database.DeleteMeetingNoteParams{
		MeetingID: meetingID, UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("delete meeting note: %w", err)
	}
	if changed > 0 {
		if err := q.TouchMeeting(ctx, meetingID); err != nil {
			return false, fmt.Errorf("touch meeting after note deletion: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit meeting note deletion: %w", err)
	}
	return changed > 0, nil
}

func authorizeMutation(
	ctx context.Context,
	q *database.Queries,
	userID, meetingID uuid.UUID,
) error {
	meeting, err := q.GetMeetingForNoteMutation(ctx, database.GetMeetingForNoteMutationParams{
		ID: meetingID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("authorize meeting note mutation: %w", err)
	}
	if meeting.Archived || (meeting.State != "draft" && meeting.State != "collecting" && meeting.State != "scheduled") {
		return ErrNotEditable
	}
	return nil
}
