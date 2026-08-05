package attendance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusGoing      Status = "going"
	StatusMaybe      Status = "maybe"
	StatusNotGoing   Status = "not_going"
	StatusUnanswered Status = "unanswered"
)

var (
	ErrInvalidInput = errors.New("invalid attendance input")
	ErrNotFound     = errors.New("attendance meeting not found")
	ErrNotAvailable = errors.New("attendance is not available")
	ErrNotEditable  = errors.New("attendance is not editable")
)

type Participant struct {
	UserID      uuid.UUID  `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	Status      Status     `json:"status"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type View struct {
	ParticipantCount int           `json:"participant_count"`
	GoingCount       int           `json:"going_count"`
	MaybeCount       int           `json:"maybe_count"`
	NotGoingCount    int           `json:"not_going_count"`
	UnansweredCount  int           `json:"unanswered_count"`
	MyStatus         Status        `json:"my_status"`
	Participants     []Participant `json:"participants"`
	Limit            int           `json:"limit"`
	Offset           int           `json:"offset"`
}

type RespondInput struct {
	Status Status `json:"status"`
}

type Repository interface {
	View(context.Context, uuid.UUID, uuid.UUID, int, int) (View, error)
	SetStatus(context.Context, uuid.UUID, uuid.UUID, Status) (bool, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Get(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	limit, offset int,
) (View, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 10_000 {
		return View{}, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	return s.repository.View(ctx, userID, meetingID, limit, offset)
}

func (s *Service) Respond(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	input RespondInput,
) (bool, error) {
	switch input.Status {
	case StatusGoing, StatusMaybe, StatusNotGoing, StatusUnanswered:
		return s.repository.SetStatus(ctx, userID, meetingID, input.Status)
	default:
		return false, fmt.Errorf("%w: unknown status", ErrInvalidInput)
	}
}
