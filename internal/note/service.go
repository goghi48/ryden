package note

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput = errors.New("invalid meeting note input")
	ErrNotFound     = errors.New("meeting note not found")
	ErrNotEditable  = errors.New("meeting notes are not editable")
)

type Note struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Text        string    `json:"text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Page struct {
	Items  []Note `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type UpsertInput struct {
	Text string `json:"text"`
}

type Repository interface {
	List(context.Context, uuid.UUID, uuid.UUID, int, int) (Page, error)
	Upsert(context.Context, uuid.UUID, uuid.UUID, string) (bool, error)
	Delete(context.Context, uuid.UUID, uuid.UUID) (bool, error)
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

func (s *Service) Upsert(
	ctx context.Context,
	userID, meetingID uuid.UUID,
	input UpsertInput,
) (bool, error) {
	input.Text = strings.TrimSpace(input.Text)
	if utf8.RuneCountInString(input.Text) < 1 || utf8.RuneCountInString(input.Text) > 200 {
		return false, fmt.Errorf("%w: text must contain 1–200 characters", ErrInvalidInput)
	}
	return s.repository.Upsert(ctx, userID, meetingID, input.Text)
}

func (s *Service) Delete(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) (bool, error) {
	return s.repository.Delete(ctx, userID, meetingID)
}
