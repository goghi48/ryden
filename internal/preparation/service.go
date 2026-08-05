package preparation

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

const (
	StatusOpen      = "open"
	StatusCompleted = "completed"
)

var (
	ErrInvalidInput        = errors.New("invalid preparation input")
	ErrNotFound            = errors.New("preparation resource not found")
	ErrNotEditable         = errors.New("preparation is not editable")
	ErrLimit               = errors.New("requirement limit reached")
	ErrDuplicate           = errors.New("requirement name already exists")
	ErrQuantityExceeded    = errors.New("claimed quantity exceeds required quantity")
	ErrNotFullyClaimed     = errors.New("requirement is not fully claimed")
	ErrHasClaims           = errors.New("requirement has active claims")
	ErrIdempotencyConflict = errors.New("idempotency key was reused with another request")
)

type Assignee struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Quantity    int32     `json:"quantity"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Requirement struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	RequiredQuantity  int32      `json:"required_quantity"`
	ClaimedQuantity   int32      `json:"claimed_quantity"`
	RemainingQuantity int32      `json:"remaining_quantity"`
	Status            string     `json:"status"`
	MyQuantity        int32      `json:"my_quantity"`
	Assignees         []Assignee `json:"assignees"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Page struct {
	Items          []Requirement `json:"items"`
	Total          int32         `json:"total"`
	OpenCount      int32         `json:"open_count"`
	CompletedCount int32         `json:"completed_count"`
	Limit          int           `json:"limit"`
	Offset         int           `json:"offset"`
}

type CreateInput struct {
	Name             string `json:"name"`
	RequiredQuantity int32  `json:"required_quantity"`
}

type UpdateInput struct {
	Name             string `json:"name"`
	RequiredQuantity int32  `json:"required_quantity"`
}

type ClaimInput struct {
	Quantity int32 `json:"quantity"`
}

type StatusInput struct {
	Status string `json:"status"`
}

type Repository interface {
	List(context.Context, uuid.UUID, uuid.UUID, int, int) (Page, error)
	Create(context.Context, uuid.UUID, uuid.UUID, string, []byte, CreateInput) (Requirement, bool, error)
	Update(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, UpdateInput) (bool, error)
	Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
	SetClaim(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int32) (bool, error)
	SetStatus(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (bool, error)
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
	if limit < 1 || limit > 50 || offset < 0 || offset > 10_000 {
		return Page{}, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	return s.repository.List(ctx, userID, meetingID, limit, offset)
}

func (s *Service) Create(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	idempotencyKey string,
	input CreateInput,
) (Requirement, bool, error) {
	name, err := normalizeRequirement(input.Name, input.RequiredQuantity)
	if err != nil {
		return Requirement{}, false, err
	}
	input.Name = name
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return Requirement{}, false, fmt.Errorf(
			"%w: Idempotency-Key must contain 8–128 characters",
			ErrInvalidInput,
		)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return Requirement{}, false, fmt.Errorf("encode requirement request: %w", err)
	}
	sum := sha256.Sum256(body)
	return s.repository.Create(ctx, ownerID, meetingID, idempotencyKey, sum[:], input)
}

func (s *Service) Update(
	ctx context.Context,
	ownerID, meetingID, requirementID uuid.UUID,
	input UpdateInput,
) (bool, error) {
	if requirementID == uuid.Nil {
		return false, fmt.Errorf("%w: requirement id is required", ErrInvalidInput)
	}
	name, err := normalizeRequirement(input.Name, input.RequiredQuantity)
	if err != nil {
		return false, err
	}
	input.Name = name
	return s.repository.Update(ctx, ownerID, meetingID, requirementID, input)
}

func (s *Service) Delete(
	ctx context.Context,
	ownerID, meetingID, requirementID uuid.UUID,
) (bool, error) {
	if requirementID == uuid.Nil {
		return false, fmt.Errorf("%w: requirement id is required", ErrInvalidInput)
	}
	return s.repository.Delete(ctx, ownerID, meetingID, requirementID)
}

func (s *Service) SetClaim(
	ctx context.Context,
	userID, meetingID, requirementID uuid.UUID,
	input ClaimInput,
) (bool, error) {
	if requirementID == uuid.Nil || input.Quantity < 0 || input.Quantity > 100_000 {
		return false, fmt.Errorf(
			"%w: quantity must be between 0 and 100000",
			ErrInvalidInput,
		)
	}
	return s.repository.SetClaim(ctx, userID, meetingID, requirementID, input.Quantity)
}

func (s *Service) SetStatus(
	ctx context.Context,
	ownerID, meetingID, requirementID uuid.UUID,
	input StatusInput,
) (bool, error) {
	input.Status = strings.TrimSpace(input.Status)
	if requirementID == uuid.Nil ||
		(input.Status != StatusOpen && input.Status != StatusCompleted) {
		return false, fmt.Errorf("%w: status must be open or completed", ErrInvalidInput)
	}
	return s.repository.SetStatus(ctx, ownerID, meetingID, requirementID, input.Status)
}

func normalizeRequirement(name string, requiredQuantity int32) (string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 120 {
		return "", fmt.Errorf("%w: name must contain 1–120 characters", ErrInvalidInput)
	}
	if requiredQuantity < 1 || requiredQuantity > 100_000 {
		return "", fmt.Errorf(
			"%w: required_quantity must be between 1 and 100000",
			ErrInvalidInput,
		)
	}
	return name, nil
}
