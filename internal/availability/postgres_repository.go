package availability

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

func (r *PostgresRepository) Snapshot(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) (Snapshot, error) {
	q := database.New(r.pool)
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	} else if err != nil {
		return Snapshot{}, fmt.Errorf("authorize availability view: %w", err)
	}
	plans, err := q.ListPlanOptions(ctx, meetingID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list availability plans: %w", err)
	}
	times, err := q.ListTimeOptions(ctx, meetingID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list availability times: %w", err)
	}
	participants, err := q.ListMeetingParticipants(ctx, meetingID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list availability participants: %w", err)
	}
	votes, err := q.ListAvailabilityVotes(ctx, meetingID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list availability responses: %w", err)
	}
	result := Snapshot{
		Plans:        make([]PlanOption, 0, len(plans)),
		Times:        make([]TimeOption, 0, len(times)),
		Participants: make([]Participant, 0, len(participants)),
		Votes:        make([]Vote, 0, len(votes)),
	}
	for _, plan := range plans {
		result.Plans = append(result.Plans, PlanOption{ID: plan.ID, Title: plan.Title})
	}
	for _, option := range times {
		result.Times = append(result.Times, TimeOption{
			ID: option.ID, PlanOptionID: option.PlanOptionID,
			StartsAt: option.StartsAt, EndsAt: option.EndsAt, Position: option.Position,
		})
	}
	for _, participant := range participants {
		result.Participants = append(result.Participants, Participant{
			UserID: participant.UserID, DisplayName: participant.DisplayName, Role: participant.Role,
		})
	}
	for _, vote := range votes {
		result.Votes = append(result.Votes, Vote{
			TimeOptionID: vote.TimeOptionID, UserID: vote.UserID, Status: Status(vote.Status),
		})
	}
	return result, nil
}

func (r *PostgresRepository) SetStatus(
	ctx context.Context,
	userID, timeOptionID uuid.UUID,
	status Status,
) (bool, error) {
	q := database.New(r.pool)
	meetingID, err := q.GetTimeOptionMeetingID(ctx, timeOptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("find availability meeting: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin availability response: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := database.New(tx)
	locked, err := tq.LockMeetingByID(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock availability meeting: %w", err)
	}
	if locked.State != "collecting" {
		return false, ErrNotEditable
	}
	if _, err := tq.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, fmt.Errorf("authorize availability response: %w", err)
	}

	var affected int64
	if status == StatusUnanswered {
		affected, err = tq.DeleteAvailabilityVote(ctx, database.DeleteAvailabilityVoteParams{
			MeetingID: meetingID, TimeOptionID: timeOptionID, UserID: userID,
		})
	} else {
		affected, err = tq.UpsertAvailabilityVote(ctx, database.UpsertAvailabilityVoteParams{
			MeetingID: meetingID, TimeOptionID: timeOptionID, UserID: userID, Status: string(status),
		})
	}
	if err != nil {
		return false, fmt.Errorf("store availability response: %w", err)
	}
	if affected > 0 {
		if err := tq.TouchMeeting(ctx, meetingID); err != nil {
			return false, fmt.Errorf("touch availability meeting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit availability response: %w", err)
	}
	return affected > 0, nil
}
