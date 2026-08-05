package decision

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return Page{}, fmt.Errorf("begin plan vote snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	} else if err != nil {
		return Page{}, fmt.Errorf("authorize plan vote list: %w", err)
	}
	options, err := q.ListPlanVoteOptions(ctx, database.ListPlanVoteOptionsParams{
		MeetingID: meetingID, UserID: userID,
	})
	if err != nil {
		return Page{}, fmt.Errorf("list plan vote options: %w", err)
	}
	responses, err := q.ListPlanVoteResponses(ctx, meetingID)
	if err != nil {
		return Page{}, fmt.Errorf("list plan vote responses: %w", err)
	}
	history, err := q.ListPlanVoteHistory(ctx, database.ListPlanVoteHistoryParams{
		MeetingID: meetingID, Limit: int32(limit), Offset: int32(offset),
	})
	if err != nil {
		return Page{}, fmt.Errorf("list plan vote history: %w", err)
	}
	participantCount, err := q.CountActiveMeetingParticipants(ctx, meetingID)
	if err != nil {
		return Page{}, fmt.Errorf("count plan vote participants: %w", err)
	}
	historyTotal, err := q.CountPlanVoteHistory(ctx, meetingID)
	if err != nil {
		return Page{}, fmt.Errorf("count plan vote history: %w", err)
	}
	result := Page{
		Options:          make([]Option, 0, len(options)),
		Responses:        make([]Response, 0, len(responses)),
		History:          make([]HistoryEntry, 0, len(history)),
		ParticipantCount: participantCount,
		AnsweredCount:    int32(len(responses)),
		HistoryTotal:     historyTotal,
		Limit:            limit,
		Offset:           offset,
	}
	for _, option := range options {
		result.Options = append(result.Options, Option{
			ID: option.ID, Title: option.Title, Description: option.Description,
			Position: option.Position, VoteCount: option.VoteCount,
			SelectedByUser: option.SelectedByUser,
		})
	}
	for _, response := range responses {
		result.Responses = append(result.Responses, Response{
			UserID: response.UserID, DisplayName: response.DisplayName,
			PlanOptionID: response.PlanOptionID, UpdatedAt: response.UpdatedAt,
		})
	}
	for _, entry := range history {
		result.History = append(result.History, HistoryEntry{
			ID: entry.ID, UserID: entry.UserID, DisplayName: entry.DisplayName,
			Action:               entry.Action,
			PreviousPlanOptionID: entry.PreviousPlanOptionID,
			PreviousPlanTitle:    textPointer(entry.PreviousPlanTitle),
			NewPlanOptionID:      entry.NewPlanOptionID,
			NewPlanTitle:         textPointer(entry.NewPlanTitle),
			CreatedAt:            entry.CreatedAt,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("commit plan vote snapshot: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) Vote(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	planOptionID *uuid.UUID,
) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin plan vote: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	locked, err := q.LockMeetingByID(ctx, meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock plan vote meeting: %w", err)
	}
	if locked.State != "collecting" {
		return false, ErrNotEditable
	}
	if _, err := q.GetMeetingForUser(ctx, database.GetMeetingForUserParams{
		ID: meetingID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, fmt.Errorf("authorize plan vote: %w", err)
	}

	current, err := q.GetCurrentPlanVote(ctx, database.GetCurrentPlanVoteParams{
		MeetingID: meetingID, UserID: userID,
	})
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read current plan vote: %w", err)
	}
	if planOptionID == nil && !hasCurrent {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit empty plan vote replay: %w", err)
		}
		return false, nil
	}

	var nextTitle string
	if planOptionID != nil {
		next, err := q.GetPlanOptionForVote(ctx, database.GetPlanOptionForVoteParams{
			MeetingID: meetingID, ID: *planOptionID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("%w: plan option does not belong to meeting", ErrInvalidInput)
		}
		if err != nil {
			return false, fmt.Errorf("validate plan vote option: %w", err)
		}
		nextTitle = next.Title
		if hasCurrent && current.PlanOptionID == *planOptionID {
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit plan vote replay: %w", err)
			}
			return false, nil
		}
	}

	action := "cast"
	var previousID *uuid.UUID
	var previousTitle *string
	if hasCurrent {
		currentID := current.PlanOptionID
		currentTitle := current.Title
		previousID = &currentID
		previousTitle = &currentTitle
		action = "change"
	}
	var nextID *uuid.UUID
	var nextTitlePointer *string
	if planOptionID == nil {
		action = "retract"
		if _, err := q.DeletePlanVote(ctx, database.DeletePlanVoteParams{
			MeetingID: meetingID, UserID: userID,
		}); err != nil {
			return false, fmt.Errorf("retract plan vote: %w", err)
		}
	} else {
		value := *planOptionID
		title := nextTitle
		nextID = &value
		nextTitlePointer = &title
		if err := q.UpsertPlanVote(ctx, database.UpsertPlanVoteParams{
			MeetingID: meetingID, UserID: userID, PlanOptionID: value,
		}); err != nil {
			return false, fmt.Errorf("store plan vote: %w", err)
		}
	}
	if err := q.CreatePlanVoteHistory(ctx, database.CreatePlanVoteHistoryParams{
		ID: uuid.New(), MeetingID: meetingID, UserID: userID, Action: action,
		PreviousPlanOptionID: previousID, PreviousPlanTitle: optionalText(previousTitle),
		NewPlanOptionID: nextID, NewPlanTitle: optionalText(nextTitlePointer),
	}); err != nil {
		return false, fmt.Errorf("append plan vote history: %w", err)
	}
	if err := q.TouchMeeting(ctx, meetingID); err != nil {
		return false, fmt.Errorf("touch plan vote meeting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit plan vote: %w", err)
	}
	return true, nil
}

func (r *PostgresRepository) Finalize(
	ctx context.Context,
	ownerID, meetingID, planOptionID, timeOptionID uuid.UUID,
) (FinalDecision, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return FinalDecision{}, false, fmt.Errorf("begin final decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	meetingRow, err := q.GetMeetingDecisionForOwnerForUpdate(
		ctx,
		database.GetMeetingDecisionForOwnerForUpdateParams{ID: meetingID, OwnerID: ownerID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return FinalDecision{}, false, ErrNotFound
	}
	if err != nil {
		return FinalDecision{}, false, fmt.Errorf("lock final decision meeting: %w", err)
	}
	if meetingRow.State == "scheduled" {
		if !sameDecision(
			meetingRow.SelectedPlanOptionID,
			meetingRow.SelectedTimeOptionID,
			planOptionID,
			timeOptionID,
		) {
			return FinalDecision{}, false, ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return FinalDecision{}, false, fmt.Errorf("commit final decision replay: %w", err)
		}
		return FinalDecision{
			PlanOptionID: planOptionID, TimeOptionID: timeOptionID, State: "scheduled",
		}, true, nil
	}
	if meetingRow.State != "collecting" {
		return FinalDecision{}, false, ErrNotEditable
	}
	if _, err := q.GetPlanOptionForVote(ctx, database.GetPlanOptionForVoteParams{
		MeetingID: meetingID, ID: planOptionID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return FinalDecision{}, false, fmt.Errorf("%w: plan option does not belong to meeting", ErrInvalidInput)
	} else if err != nil {
		return FinalDecision{}, false, fmt.Errorf("validate final plan option: %w", err)
	}
	timeRow, err := q.GetTimeOptionForDecision(ctx, database.GetTimeOptionForDecisionParams{
		MeetingID: meetingID, ID: timeOptionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return FinalDecision{}, false, fmt.Errorf("%w: time option does not belong to meeting", ErrInvalidInput)
	}
	if err != nil {
		return FinalDecision{}, false, fmt.Errorf("validate final time option: %w", err)
	}
	if !compatible(planOptionID, timeRow.PlanOptionID) {
		return FinalDecision{}, false, ErrIncompatible
	}
	if err := q.ScheduleMeeting(ctx, database.ScheduleMeetingParams{
		ID: meetingID, SelectedPlanOptionID: &planOptionID, SelectedTimeOptionID: &timeOptionID,
	}); err != nil {
		return FinalDecision{}, false, fmt.Errorf("schedule meeting: %w", err)
	}
	if _, err := q.RevokeActiveInvitations(ctx, meetingID); err != nil {
		return FinalDecision{}, false, fmt.Errorf("revoke invitations after final decision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return FinalDecision{}, false, fmt.Errorf("commit final decision: %w", err)
	}
	return FinalDecision{
		PlanOptionID: planOptionID, TimeOptionID: timeOptionID, State: "scheduled",
	}, false, nil
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
