package friendship

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("friendship not found")
)

type Relationship string

const (
	RelationshipNone     Relationship = "none"
	RelationshipOutgoing Relationship = "outgoing"
	RelationshipIncoming Relationship = "incoming"
	RelationshipFriend   Relationship = "friend"
)

type Person struct {
	ID             uuid.UUID    `json:"id"`
	Nickname       string       `json:"nickname"`
	DisplayName    string       `json:"display_name"`
	AvatarURL      *string      `json:"avatar_url"`
	AvatarRevision *int64       `json:"avatar_revision"`
	Relationship   Relationship `json:"relationship"`
	RequestID      *uuid.UUID   `json:"request_id,omitempty"`
}

type Item struct {
	RequestID      uuid.UUID `json:"request_id"`
	UserID         uuid.UUID `json:"user_id"`
	Nickname       string    `json:"nickname"`
	DisplayName    string    `json:"display_name"`
	AvatarURL      *string   `json:"avatar_url"`
	AvatarRevision *int64    `json:"avatar_revision"`
	ChangedAt      time.Time `json:"changed_at"`
}

type Page struct {
	Items  []Item `json:"items"`
	Total  int64  `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type Overview struct {
	Friends  Page `json:"friends"`
	Incoming Page `json:"incoming"`
	Outgoing Page `json:"outgoing"`
}

type Mutation struct {
	Changed bool `json:"changed"`
}

type Repository interface {
	Search(context.Context, uuid.UUID, string, int) ([]Person, error)
	Overview(context.Context, uuid.UUID, int, int) (Overview, error)
	Send(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	Accept(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	DeleteRequest(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	RemoveFriend(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Search(ctx context.Context, userID uuid.UUID, rawPrefix string, limit int) ([]Person, error) {
	prefix, err := normalizeSearchPrefix(rawPrefix)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 20 {
		return nil, fmt.Errorf("%w: search limit must not exceed 20", ErrInvalidInput)
	}
	return s.repository.Search(ctx, userID, prefix, limit)
}

func (s *Service) Overview(ctx context.Context, userID uuid.UUID, limit, offset int) (Overview, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 || offset < 0 || offset > 10_000 {
		return Overview{}, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	return s.repository.Overview(ctx, userID, limit, offset)
}

func (s *Service) Send(ctx context.Context, userID, targetUserID uuid.UUID) (Mutation, error) {
	if targetUserID == uuid.Nil || targetUserID == userID {
		return Mutation{}, fmt.Errorf("%w: choose another user", ErrInvalidInput)
	}
	changed, err := s.repository.Send(ctx, userID, targetUserID)
	return Mutation{Changed: changed}, err
}

func (s *Service) Accept(ctx context.Context, userID, requestID uuid.UUID) (Mutation, error) {
	if requestID == uuid.Nil {
		return Mutation{}, fmt.Errorf("%w: invalid request", ErrInvalidInput)
	}
	changed, err := s.repository.Accept(ctx, userID, requestID)
	return Mutation{Changed: changed}, err
}

func (s *Service) DeleteRequest(ctx context.Context, userID, requestID uuid.UUID) (Mutation, error) {
	if requestID == uuid.Nil {
		return Mutation{}, fmt.Errorf("%w: invalid request", ErrInvalidInput)
	}
	changed, err := s.repository.DeleteRequest(ctx, userID, requestID)
	return Mutation{Changed: changed}, err
}

func (s *Service) RemoveFriend(ctx context.Context, userID, friendID uuid.UUID) (Mutation, error) {
	if friendID == uuid.Nil || friendID == userID {
		return Mutation{}, fmt.Errorf("%w: invalid friend", ErrInvalidInput)
	}
	changed, err := s.repository.RemoveFriend(ctx, userID, friendID)
	return Mutation{Changed: changed}, err
}

func normalizeSearchPrefix(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) < 3 || len(value) > 24 {
		return "", fmt.Errorf("%w: enter 3 to 24 nickname characters", ErrInvalidInput)
	}
	if value[0] < 'a' || value[0] > 'z' {
		return "", fmt.Errorf("%w: nickname starts with a Latin letter", ErrInvalidInput)
	}
	previousUnderscore := false
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' || character == '_'
		if !valid || character == '_' && previousUnderscore {
			return "", fmt.Errorf("%w: use Latin letters, digits, and single underscores", ErrInvalidInput)
		}
		previousUnderscore = character == '_'
	}
	return value, nil
}
