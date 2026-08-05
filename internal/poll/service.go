package poll

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput        = errors.New("invalid poll input")
	ErrNotFound            = errors.New("poll not found")
	ErrNotEditable         = errors.New("poll is not editable")
	ErrLimit               = errors.New("poll limit reached")
	ErrClosed              = errors.New("poll is closed")
	ErrDeadline            = errors.New("poll deadline has passed")
	ErrRevoteDisabled      = errors.New("poll answer cannot be changed")
	ErrConflict            = errors.New("poll decision conflicts with current state")
	ErrIdempotencyConflict = errors.New("idempotency key was reused with another request")
)

type Option struct {
	ID             uuid.UUID `json:"id"`
	Label          string    `json:"label"`
	Position       int16     `json:"position"`
	VoteCount      int32     `json:"vote_count"`
	SelectedByUser bool      `json:"selected_by_user"`
	Voters         []Voter   `json:"voters"`
}

type Voter struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Poll struct {
	ID               uuid.UUID  `json:"id"`
	CreatedByUserID  uuid.UUID  `json:"created_by_user_id"`
	Question         string     `json:"question"`
	ResponseMode     string     `json:"response_mode"`
	IsAnonymous      bool       `json:"is_anonymous"`
	AllowRevote      bool       `json:"allow_revote"`
	CanManage        bool       `json:"can_manage"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	State            string     `json:"state"`
	SelectedOptionID *uuid.UUID `json:"selected_option_id,omitempty"`
	AcceptingAnswers bool       `json:"accepting_answers"`
	Options          []Option   `json:"options"`
	ParticipantCount int32      `json:"participant_count"`
	RespondentCount  int32      `json:"respondent_count"`
	TotalSelections  int32      `json:"total_selections"`
	CreatedAt        time.Time  `json:"created_at"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
}

type CreateInput struct {
	Question     string     `json:"question"`
	ResponseMode string     `json:"response_mode"`
	IsAnonymous  bool       `json:"is_anonymous"`
	AllowRevote  bool       `json:"allow_revote"`
	Deadline     *time.Time `json:"deadline"`
	Options      []string   `json:"options"`
}

type VoteInput struct {
	OptionIDs []uuid.UUID `json:"option_ids"`
}

type CloseInput struct {
	SelectedOptionID *uuid.UUID `json:"selected_option_id"`
}

type HistoryEntry struct {
	ID                   uuid.UUID   `json:"id"`
	UserID               uuid.UUID   `json:"user_id"`
	DisplayName          string      `json:"display_name"`
	Action               string      `json:"action"`
	PreviousOptionIDs    []uuid.UUID `json:"previous_option_ids"`
	PreviousOptionLabels []string    `json:"previous_option_labels"`
	NewOptionIDs         []uuid.UUID `json:"new_option_ids"`
	NewOptionLabels      []string    `json:"new_option_labels"`
	CreatedAt            time.Time   `json:"created_at"`
}

type HistoryPage struct {
	Items  []HistoryEntry `json:"items"`
	Total  int32          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type Repository interface {
	Create(context.Context, uuid.UUID, uuid.UUID, string, []byte, CreateInput) (Poll, bool, error)
	List(context.Context, uuid.UUID, uuid.UUID) ([]Poll, error)
	History(context.Context, uuid.UUID, uuid.UUID, int, int) (HistoryPage, error)
	Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	Vote(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (bool, error)
	Close(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *uuid.UUID) (bool, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) Create(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	idempotencyKey string,
	input CreateInput,
) (Poll, bool, error) {
	input.Question = strings.TrimSpace(input.Question)
	input.ResponseMode = strings.TrimSpace(input.ResponseMode)
	if utf8.RuneCountInString(input.Question) < 1 || utf8.RuneCountInString(input.Question) > 200 {
		return Poll{}, false, fmt.Errorf("%w: question must contain 1–200 characters", ErrInvalidInput)
	}
	if input.ResponseMode != "single" && input.ResponseMode != "multiple" {
		return Poll{}, false, fmt.Errorf("%w: response_mode must be single or multiple", ErrInvalidInput)
	}
	if len(input.Options) < 2 || len(input.Options) > 10 {
		return Poll{}, false, fmt.Errorf("%w: poll must contain 2–10 options", ErrInvalidInput)
	}
	seen := make(map[string]struct{}, len(input.Options))
	for index := range input.Options {
		input.Options[index] = strings.TrimSpace(input.Options[index])
		if utf8.RuneCountInString(input.Options[index]) < 1 ||
			utf8.RuneCountInString(input.Options[index]) > 120 {
			return Poll{}, false, fmt.Errorf("%w: every option must contain 1–120 characters", ErrInvalidInput)
		}
		key := strings.ToLower(input.Options[index])
		if _, exists := seen[key]; exists {
			return Poll{}, false, fmt.Errorf("%w: poll options must be unique", ErrInvalidInput)
		}
		seen[key] = struct{}{}
	}
	if input.Deadline != nil {
		normalized := input.Deadline.UTC()
		if !normalized.After(s.now().UTC()) {
			return Poll{}, false, fmt.Errorf("%w: deadline must be in the future", ErrInvalidInput)
		}
		input.Deadline = &normalized
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return Poll{}, false, fmt.Errorf("%w: Idempotency-Key must contain 8–128 characters", ErrInvalidInput)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return Poll{}, false, fmt.Errorf("encode poll request: %w", err)
	}
	sum := sha256.Sum256(body)
	return s.repository.Create(ctx, userID, meetingID, idempotencyKey, sum[:], input)
}

func (s *Service) List(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) ([]Poll, error) {
	return s.repository.List(ctx, userID, meetingID)
}

func (s *Service) History(
	ctx context.Context,
	userID, pollID uuid.UUID,
	limit, offset int,
) (HistoryPage, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1_000_000 {
		return HistoryPage{}, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	return s.repository.History(ctx, userID, pollID, limit, offset)
}

func (s *Service) Delete(
	ctx context.Context,
	ownerID, meetingID, pollID uuid.UUID,
) error {
	return s.repository.Delete(ctx, ownerID, meetingID, pollID)
}

func (s *Service) Vote(
	ctx context.Context,
	userID, pollID uuid.UUID,
	input VoteInput,
) (bool, error) {
	if len(input.OptionIDs) > 10 {
		return false, fmt.Errorf("%w: too many selected options", ErrInvalidInput)
	}
	seen := make(map[uuid.UUID]struct{}, len(input.OptionIDs))
	for _, optionID := range input.OptionIDs {
		if optionID == uuid.Nil {
			return false, fmt.Errorf("%w: invalid option id", ErrInvalidInput)
		}
		if _, exists := seen[optionID]; exists {
			return false, fmt.Errorf("%w: option ids must be unique", ErrInvalidInput)
		}
		seen[optionID] = struct{}{}
	}
	return s.repository.Vote(ctx, userID, pollID, input.OptionIDs)
}

func (s *Service) Close(
	ctx context.Context,
	ownerID, meetingID, pollID uuid.UUID,
	input CloseInput,
) (bool, error) {
	if input.SelectedOptionID != nil && *input.SelectedOptionID == uuid.Nil {
		return false, fmt.Errorf("%w: selected_option_id must be a valid UUID or null", ErrInvalidInput)
	}
	return s.repository.Close(ctx, ownerID, meetingID, pollID, input.SelectedOptionID)
}
