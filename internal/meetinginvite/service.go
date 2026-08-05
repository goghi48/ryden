package meetinginvite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("meeting invitation not found")
	ErrConflict     = errors.New("meeting invitation conflict")
)

const MaxBatchSize = 50

type Candidate struct {
	UserID           uuid.UUID  `json:"user_id"`
	Nickname         string     `json:"nickname"`
	DisplayName      string     `json:"display_name"`
	AvatarURL        *string    `json:"avatar_url"`
	AvatarRevision   *int64     `json:"avatar_revision"`
	InvitationID     *uuid.UUID `json:"invitation_id"`
	InvitationStatus *string    `json:"invitation_status"`
	IsParticipant    bool       `json:"is_participant"`
}

type CandidatePage struct {
	Items  []Candidate `json:"items"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

type Incoming struct {
	ID               uuid.UUID  `json:"id"`
	MeetingID        uuid.UUID  `json:"meeting_id"`
	MeetingTitle     string     `json:"meeting_title"`
	OwnerDisplayName string     `json:"owner_display_name"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
	Timezone         string     `json:"timezone"`
	CreatedAt        time.Time  `json:"created_at"`
}

type IncomingPage struct {
	Items  []Incoming `json:"items"`
	Total  int64      `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type SendMutation struct {
	ChangedCount int `json:"changed_count"`
}

type ResponseMutation struct {
	MeetingID uuid.UUID `json:"meeting_id"`
	Changed   bool      `json:"changed"`
	Joined    bool      `json:"joined"`
}

type Repository interface {
	Candidates(context.Context, uuid.UUID, uuid.UUID, int, int) (CandidatePage, error)
	Incoming(context.Context, uuid.UUID, int, int) (IncomingPage, error)
	Send(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (int, error)
	Accept(context.Context, uuid.UUID, uuid.UUID) (ResponseMutation, error)
	Decline(context.Context, uuid.UUID, uuid.UUID) (ResponseMutation, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Candidates(ctx context.Context, ownerID, meetingID uuid.UUID, limit, offset int) (CandidatePage, error) {
	limit, offset, err := pagination(limit, offset)
	if err != nil {
		return CandidatePage{}, err
	}
	return s.repository.Candidates(ctx, ownerID, meetingID, limit, offset)
}

func (s *Service) Incoming(ctx context.Context, userID uuid.UUID, limit, offset int) (IncomingPage, error) {
	limit, offset, err := pagination(limit, offset)
	if err != nil {
		return IncomingPage{}, err
	}
	return s.repository.Incoming(ctx, userID, limit, offset)
}

func (s *Service) Send(ctx context.Context, ownerID, meetingID uuid.UUID, userIDs []uuid.UUID) (SendMutation, error) {
	if meetingID == uuid.Nil || len(userIDs) == 0 || len(userIDs) > MaxBatchSize {
		return SendMutation{}, fmt.Errorf("%w: choose 1 to %d friends", ErrInvalidInput, MaxBatchSize)
	}
	unique := make([]uuid.UUID, 0, len(userIDs))
	seen := make(map[uuid.UUID]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID == uuid.Nil || userID == ownerID {
			return SendMutation{}, fmt.Errorf("%w: invalid invitee", ErrInvalidInput)
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		unique = append(unique, userID)
	}
	changed, err := s.repository.Send(ctx, ownerID, meetingID, unique)
	return SendMutation{ChangedCount: changed}, err
}

func (s *Service) Accept(ctx context.Context, userID, invitationID uuid.UUID) (ResponseMutation, error) {
	if invitationID == uuid.Nil {
		return ResponseMutation{}, fmt.Errorf("%w: invalid invitation", ErrInvalidInput)
	}
	return s.repository.Accept(ctx, userID, invitationID)
}

func (s *Service) Decline(ctx context.Context, userID, invitationID uuid.UUID) (ResponseMutation, error) {
	if invitationID == uuid.Nil {
		return ResponseMutation{}, fmt.Errorf("%w: invalid invitation", ErrInvalidInput)
	}
	return s.repository.Decline(ctx, userID, invitationID)
}

func pagination(limit, offset int) (int, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 50 || offset < 0 || offset > 10_000 {
		return 0, 0, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	return limit, offset, nil
}
