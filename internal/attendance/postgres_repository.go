package attendance

import (
	"context"
	"errors"
	"fmt"

	"github.com/goghi48/ryden/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) View(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	limit, offset int,
) (View, error) {
	q := database.New(r.pool)
	meeting, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("authorize attendance view: %w", err)
	}
	if meeting.CoordinationMode != "fixed" {
		return View{}, ErrNotAvailable
	}
	counts, err := q.CountAttendanceParticipants(ctx, meetingID)
	if err != nil {
		return View{}, fmt.Errorf("count attendance participants: %w", err)
	}
	rows, err := q.ListAttendanceParticipants(ctx, database.ListAttendanceParticipantsParams{
		MeetingID: meetingID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return View{}, fmt.Errorf("list attendance participants: %w", err)
	}
	result := View{
		ParticipantCount: int(counts.ParticipantCount),
		GoingCount:       int(counts.GoingCount),
		MaybeCount:       int(counts.MaybeCount),
		NotGoingCount:    int(counts.NotGoingCount),
		UnansweredCount:  int(counts.UnansweredCount),
		MyStatus:         StatusUnanswered,
		Participants:     make([]Participant, 0, len(rows)),
		Limit:            limit,
		Offset:           offset,
	}
	if current, err := q.GetAttendanceResponse(ctx, database.GetAttendanceResponseParams{
		MeetingID: meetingID, UserID: userID,
	}); err == nil {
		result.MyStatus = Status(current)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return View{}, fmt.Errorf("read current attendance response: %w", err)
	}
	for _, row := range rows {
		status := Status(row.Status)
		switch status {
		case StatusGoing, StatusMaybe, StatusNotGoing:
		default:
			status = StatusUnanswered
		}
		result.Participants = append(result.Participants, Participant{
			UserID: row.UserID, DisplayName: row.DisplayName, Role: row.Role,
			Status: status, UpdatedAt: row.UpdatedAt,
		})
	}
	return result, nil
}

func (r *PostgresRepository) SetStatus(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	status Status,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin attendance response: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)

	locked, err := q.LockMeetingByID(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock attendance meeting: %w", err)
	}
	if locked.CoordinationMode != "fixed" {
		return false, ErrNotAvailable
	}
	if locked.State != "scheduled" {
		return false, ErrNotEditable
	}
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, fmt.Errorf("authorize attendance response: %w", err)
	}

	previous, err := q.GetAttendanceResponse(ctx, database.GetAttendanceResponseParams{
		MeetingID: meetingID, UserID: userID,
	})
	hasPrevious := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read attendance response: %w", err)
	}
	if (status == StatusUnanswered && !hasPrevious) || (hasPrevious && previous == string(status)) {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit attendance no-op: %w", err)
		}
		return false, nil
	}

	if status == StatusUnanswered {
		if _, err := q.DeleteAttendanceResponse(ctx, database.DeleteAttendanceResponseParams{
			MeetingID: meetingID, UserID: userID,
		}); err != nil {
			return false, fmt.Errorf("delete attendance response: %w", err)
		}
	} else if err := q.UpsertAttendanceResponse(ctx, database.UpsertAttendanceResponseParams{
		MeetingID: meetingID, UserID: userID, Status: string(status),
	}); err != nil {
		return false, fmt.Errorf("store attendance response: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch attendance meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit attendance response: %w", err)
	}
	return true, nil
}
