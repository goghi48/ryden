package decision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid decision input")
	ErrNotFound     = errors.New("meeting decision not found")
	ErrNotEditable  = errors.New("meeting decision is not editable")
	ErrIncompatible = errors.New("plan and time options are incompatible")
	ErrConflict     = errors.New("meeting has another final decision")
)

type Option struct {
	ID             uuid.UUID `json:"id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Position       int16     `json:"position"`
	VoteCount      int32     `json:"vote_count"`
	SelectedByUser bool      `json:"selected_by_user"`
}

type Response struct {
	UserID       uuid.UUID `json:"user_id"`
	DisplayName  string    `json:"display_name"`
	PlanOptionID uuid.UUID `json:"plan_option_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type HistoryEntry struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               uuid.UUID  `json:"user_id"`
	DisplayName          string     `json:"display_name"`
	Action               string     `json:"action"`
	PreviousPlanOptionID *uuid.UUID `json:"previous_plan_option_id"`
	PreviousPlanTitle    *string    `json:"previous_plan_title"`
	NewPlanOptionID      *uuid.UUID `json:"new_plan_option_id"`
	NewPlanTitle         *string    `json:"new_plan_title"`
	CreatedAt            time.Time  `json:"created_at"`
}

type Page struct {
	Options          []Option       `json:"options"`
	Responses        []Response     `json:"responses"`
	History          []HistoryEntry `json:"history"`
	ParticipantCount int32          `json:"participant_count"`
	AnsweredCount    int32          `json:"answered_count"`
	HistoryTotal     int32          `json:"history_total"`
	Limit            int            `json:"limit"`
	Offset           int            `json:"offset"`
}

type VoteInput struct {
	PlanOptionID *uuid.UUID `json:"plan_option_id"`
}

type FinalizeInput struct {
	PlanOptionID uuid.UUID `json:"plan_option_id"`
	TimeOptionID uuid.UUID `json:"time_option_id"`
}

type FinalDecision struct {
	PlanOptionID uuid.UUID `json:"plan_option_id"`
	TimeOptionID uuid.UUID `json:"time_option_id"`
	State        string    `json:"state"`
}

type Repository interface {
	List(context.Context, uuid.UUID, uuid.UUID, int, int) (Page, error)
	Vote(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (bool, error)
	Finalize(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (FinalDecision, bool, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	limit, offset int,
) (Page, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 10_000 {
		return Page{}, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	return s.repository.List(ctx, userID, meetingID, limit, offset)
}

func (s *Service) Vote(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	input VoteInput,
) (bool, error) {
	if input.PlanOptionID != nil && *input.PlanOptionID == uuid.Nil {
		return false, fmt.Errorf("%w: invalid plan_option_id", ErrInvalidInput)
	}
	return s.repository.Vote(ctx, userID, meetingID, input.PlanOptionID)
}

func (s *Service) Finalize(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	input FinalizeInput,
) (FinalDecision, bool, error) {
	if input.PlanOptionID == uuid.Nil || input.TimeOptionID == uuid.Nil {
		return FinalDecision{}, false, fmt.Errorf(
			"%w: plan_option_id and time_option_id are required",
			ErrInvalidInput,
		)
	}
	return s.repository.Finalize(
		ctx,
		ownerID,
		meetingID,
		input.PlanOptionID,
		input.TimeOptionID,
	)
}

func compatible(planOptionID uuid.UUID, timePlanOptionID *uuid.UUID) bool {
	return timePlanOptionID == nil || *timePlanOptionID == planOptionID
}

func sameDecision(
	selectedPlanOptionID, selectedTimeOptionID *uuid.UUID,
	planOptionID, timeOptionID uuid.UUID,
) bool {
	return selectedPlanOptionID != nil &&
		selectedTimeOptionID != nil &&
		*selectedPlanOptionID == planOptionID &&
		*selectedTimeOptionID == timeOptionID
}
