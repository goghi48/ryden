package meeting

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput          = errors.New("invalid meeting input")
	ErrNotFound              = errors.New("meeting not found")
	ErrOptionNotFound        = errors.New("meeting option not found")
	ErrIdempotencyConflict   = errors.New("idempotency key was reused with another request")
	ErrNotEditable           = errors.New("meeting setup is no longer editable")
	ErrSetupIncomplete       = errors.New("meeting setup is incomplete")
	ErrFixedSetupInvalid     = errors.New("fixed meeting setup is invalid")
	ErrOptionLimit           = errors.New("meeting option limit reached")
	ErrDuplicateOption       = errors.New("meeting option already exists")
	ErrInvitationInvalid     = errors.New("invitation is invalid, expired, or revoked")
	ErrVersionConflict       = errors.New("meeting version conflict")
	ErrNotCompletable        = errors.New("meeting cannot be completed from its current state")
	ErrPreparationIncomplete = errors.New("meeting preparation is incomplete")
	ErrNotCancellable        = errors.New("meeting cannot be cancelled from its current state")
)

var allowedEventTypes = map[string]struct{}{
	"other":      {},
	"dinner":     {},
	"game_night": {},
	"birthday":   {},
	"hike":       {},
	"trip":       {},
}

var allowedCoordinationModes = map[string]struct{}{
	"planning": {},
	"fixed":    {},
}

const maxTimeOptionDuration = 30*24*time.Hour + 23*time.Hour + 59*time.Minute

type Meeting struct {
	ID                   uuid.UUID  `json:"id"`
	OwnerID              uuid.UUID  `json:"owner_id"`
	Title                string     `json:"title"`
	Description          string     `json:"description"`
	EventType            string     `json:"event_type"`
	CoordinationMode     string     `json:"coordination_mode"`
	CoverURL             *string    `json:"cover_url"`
	HasPhoto             bool       `json:"has_photo"`
	LocationName         *string    `json:"location_name"`
	LocationURL          *string    `json:"location_url"`
	Timezone             string     `json:"timezone"`
	State                string     `json:"state"`
	SelectedPlanOptionID *uuid.UUID `json:"selected_plan_option_id,omitempty"`
	SelectedTimeOptionID *uuid.UUID `json:"selected_time_option_id,omitempty"`
	Version              int64      `json:"version"`
	ParticipantRole      string     `json:"participant_role"`
	ParticipantJoinedAt  *time.Time `json:"participant_joined_at,omitempty"`
	SelectedStartsAt     *time.Time `json:"selected_starts_at,omitempty"`
	SelectedEndsAt       *time.Time `json:"selected_ends_at,omitempty"`
	MyAttendanceStatus   *string    `json:"my_attendance_status,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type PlanOption struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	HasPhoto    bool      `json:"has_photo"`
	Position    int16     `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
}

type TimeOption struct {
	ID           uuid.UUID  `json:"id"`
	PlanOptionID *uuid.UUID `json:"plan_option_id,omitempty"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	Position     int16      `json:"position"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Participant struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
}

type Detail struct {
	Meeting
	PlanOptions               []PlanOption  `json:"plan_options"`
	TimeOptions               []TimeOption  `json:"time_options"`
	Participants              []Participant `json:"participants"`
	ActiveInvitationExpiresAt *time.Time    `json:"active_invitation_expires_at,omitempty"`
}

type CreateInput struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	EventType        string     `json:"event_type"`
	CoordinationMode string     `json:"coordination_mode"`
	CoverURL         *string    `json:"cover_url"`
	LocationName     *string    `json:"location_name"`
	LocationURL      *string    `json:"location_url"`
	Timezone         string     `json:"timezone"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
}

type UpdateInput struct {
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	EventType       string     `json:"event_type"`
	CoverURL        *string    `json:"cover_url"`
	LocationName    *string    `json:"location_name"`
	LocationURL     *string    `json:"location_url"`
	StartsAt        *time.Time `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at"`
	ExpectedVersion int64      `json:"version"`
}

type AddPlanOptionInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type UpdatePlanOptionInput struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	ExpectedVersion int64  `json:"version"`
}

type AddTimeOptionInput struct {
	PlanOptionID *uuid.UUID `json:"plan_option_id"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
}

type UpdateTimeOptionInput struct {
	PlanOptionID    *uuid.UUID `json:"plan_option_id"`
	StartsAt        time.Time  `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at"`
	ExpectedVersion int64      `json:"version"`
}

type CreateInvitationInput struct {
	Secret string `json:"secret"`
}

type JoinInvitationInput struct {
	Token string `json:"token"`
}

type Invitation struct {
	ExpiresAt time.Time `json:"expires_at"`
}

type Completion struct {
	MeetingID uuid.UUID `json:"meeting_id"`
	State     string    `json:"state"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Cancellation struct {
	MeetingID uuid.UUID `json:"meeting_id"`
	State     string    `json:"state"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Page struct {
	Items  []Meeting `json:"items"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

type Repository interface {
	Create(context.Context, uuid.UUID, string, []byte, CreateInput) (Meeting, bool, error)
	Update(context.Context, uuid.UUID, uuid.UUID, UpdateInput) (Meeting, error)
	List(context.Context, uuid.UUID, int, int) ([]Meeting, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (Detail, error)
	AddPlanOption(context.Context, uuid.UUID, uuid.UUID, string, AddPlanOptionInput) (PlanOption, bool, error)
	UpdatePlanOption(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, UpdatePlanOptionInput) (PlanOption, error)
	DeletePlanOption(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	AddTimeOption(context.Context, uuid.UUID, uuid.UUID, string, AddTimeOptionInput) (TimeOption, bool, error)
	UpdateTimeOption(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, UpdateTimeOptionInput) (TimeOption, error)
	DeleteTimeOption(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	CreateInvitation(context.Context, uuid.UUID, uuid.UUID, string, []byte) (Invitation, bool, error)
	RevokeInvitation(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	JoinInvitation(context.Context, uuid.UUID, []byte) (Detail, bool, error)
	Complete(context.Context, uuid.UUID, uuid.UUID) (Completion, bool, error)
	Cancel(context.Context, uuid.UUID, uuid.UUID) (Cancellation, bool, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(
	ctx context.Context,
	ownerID uuid.UUID,
	idempotencyKey string,
	input CreateInput,
) (Meeting, bool, error) {
	normalized, err := validateCreate(input)
	if err != nil {
		return Meeting{}, false, err
	}
	idempotencyKey, err = validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return Meeting{}, false, err
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return Meeting{}, false, fmt.Errorf("encode meeting request: %w", err)
	}
	sum := sha256.Sum256(body)
	return s.repository.Create(ctx, ownerID, idempotencyKey, sum[:], normalized)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, limit, offset int) (Page, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 10_000 {
		return Page{}, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	items, err := s.repository.List(ctx, userID, limit, offset)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	input UpdateInput,
) (Meeting, error) {
	if input.ExpectedVersion < 1 {
		return Meeting{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	normalized, err := validateMetadata(CreateInput{
		Title:        input.Title,
		Description:  input.Description,
		EventType:    input.EventType,
		CoverURL:     input.CoverURL,
		LocationName: input.LocationName,
		LocationURL:  input.LocationURL,
	})
	if err != nil {
		return Meeting{}, err
	}
	if input.StartsAt == nil && input.EndsAt != nil {
		return Meeting{}, fmt.Errorf("%w: starts_at is required when ends_at is provided", ErrInvalidInput)
	}
	if input.StartsAt != nil {
		startsAt, endsAt, err := validateTimeOption(nil, *input.StartsAt, input.EndsAt)
		if err != nil {
			return Meeting{}, err
		}
		input.StartsAt, input.EndsAt = &startsAt, endsAt
	}
	return s.repository.Update(ctx, ownerID, meetingID, UpdateInput{
		Title:           normalized.Title,
		Description:     normalized.Description,
		EventType:       normalized.EventType,
		CoverURL:        normalized.CoverURL,
		LocationName:    normalized.LocationName,
		LocationURL:     normalized.LocationURL,
		StartsAt:        input.StartsAt,
		EndsAt:          input.EndsAt,
		ExpectedVersion: input.ExpectedVersion,
	})
}

func (s *Service) Get(ctx context.Context, userID, meetingID uuid.UUID) (Detail, error) {
	return s.repository.Get(ctx, userID, meetingID)
}

func (s *Service) AddPlanOption(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	idempotencyKey string,
	input AddPlanOptionInput,
) (PlanOption, bool, error) {
	title, description, err := validatePlanOption(input.Title, input.Description)
	if err != nil {
		return PlanOption{}, false, err
	}
	input.Title, input.Description = title, description
	key, err := validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return PlanOption{}, false, err
	}
	return s.repository.AddPlanOption(ctx, ownerID, meetingID, key, input)
}

func (s *Service) UpdatePlanOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	input UpdatePlanOptionInput,
) (PlanOption, error) {
	if input.ExpectedVersion < 1 {
		return PlanOption{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	title, description, err := validatePlanOption(input.Title, input.Description)
	if err != nil {
		return PlanOption{}, err
	}
	input.Title, input.Description = title, description
	return s.repository.UpdatePlanOption(ctx, ownerID, meetingID, optionID, input)
}

func (s *Service) DeletePlanOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
) error {
	return s.repository.DeletePlanOption(ctx, ownerID, meetingID, optionID)
}

func (s *Service) AddTimeOption(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	idempotencyKey string,
	input AddTimeOptionInput,
) (TimeOption, bool, error) {
	startsAt, endsAt, err := validateTimeOption(input.PlanOptionID, input.StartsAt, input.EndsAt)
	if err != nil {
		return TimeOption{}, false, err
	}
	key, err := validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return TimeOption{}, false, err
	}
	input.StartsAt, input.EndsAt = startsAt, endsAt
	return s.repository.AddTimeOption(ctx, ownerID, meetingID, key, input)
}

func (s *Service) UpdateTimeOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	input UpdateTimeOptionInput,
) (TimeOption, error) {
	if input.ExpectedVersion < 1 {
		return TimeOption{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	startsAt, endsAt, err := validateTimeOption(input.PlanOptionID, input.StartsAt, input.EndsAt)
	if err != nil {
		return TimeOption{}, err
	}
	input.StartsAt, input.EndsAt = startsAt, endsAt
	return s.repository.UpdateTimeOption(ctx, ownerID, meetingID, optionID, input)
}

func (s *Service) DeleteTimeOption(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
) error {
	return s.repository.DeleteTimeOption(ctx, ownerID, meetingID, optionID)
}

func (s *Service) CreateInvitation(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	idempotencyKey string,
	input CreateInvitationInput,
) (Invitation, bool, error) {
	key, err := validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return Invitation{}, false, err
	}
	hash, err := invitationHash(input.Secret)
	if err != nil {
		return Invitation{}, false, err
	}
	return s.repository.CreateInvitation(ctx, ownerID, meetingID, key, hash)
}

func (s *Service) RevokeInvitation(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
) (bool, error) {
	return s.repository.RevokeInvitation(ctx, ownerID, meetingID)
}

func (s *Service) JoinInvitation(
	ctx context.Context,
	userID uuid.UUID,
	input JoinInvitationInput,
) (Detail, bool, error) {
	hash, err := invitationHash(input.Token)
	if err != nil {
		return Detail{}, false, ErrInvitationInvalid
	}
	return s.repository.JoinInvitation(ctx, userID, hash)
}

func (s *Service) Complete(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
) (Completion, bool, error) {
	return s.repository.Complete(ctx, ownerID, meetingID)
}

func (s *Service) Cancel(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
) (Cancellation, bool, error) {
	return s.repository.Cancel(ctx, ownerID, meetingID)
}

func validatePlanOption(title, description string) (string, string, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 120 {
		return "", "", fmt.Errorf("%w: title must contain 1–120 characters", ErrInvalidInput)
	}
	if utf8.RuneCountInString(description) > 500 {
		return "", "", fmt.Errorf("%w: description must contain at most 500 characters", ErrInvalidInput)
	}
	return title, description, nil
}

func validateTimeOption(
	planOptionID *uuid.UUID,
	startsAt time.Time,
	endsAt *time.Time,
) (time.Time, *time.Time, error) {
	if startsAt.IsZero() {
		return time.Time{}, nil, fmt.Errorf("%w: starts_at is required", ErrInvalidInput)
	}
	if startsAt.Second() != 0 || startsAt.Nanosecond() != 0 || startsAt.Minute()%5 != 0 {
		return time.Time{}, nil, fmt.Errorf("%w: starts_at must use a 5-minute interval", ErrInvalidInput)
	}
	if planOptionID != nil && *planOptionID == uuid.Nil {
		return time.Time{}, nil, fmt.Errorf("%w: invalid plan_option_id", ErrInvalidInput)
	}
	startsAt = startsAt.UTC()
	if endsAt == nil {
		return startsAt, nil, nil
	}
	normalizedEnd := endsAt.UTC()
	duration := normalizedEnd.Sub(startsAt)
	if duration < time.Minute || duration > maxTimeOptionDuration {
		return time.Time{}, nil, fmt.Errorf(
			"%w: duration must be between 1 minute and 30 days 23 hours 59 minutes",
			ErrInvalidInput,
		)
	}
	return startsAt, &normalizedEnd, nil
}

func validateCreate(input CreateInput) (CreateInput, error) {
	input, err := validateMetadata(input)
	if err != nil {
		return CreateInput{}, err
	}
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.Timezone == "" {
		return CreateInput{}, fmt.Errorf("%w: timezone is required", ErrInvalidInput)
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return CreateInput{}, fmt.Errorf("%w: unknown timezone", ErrInvalidInput)
	}
	input.CoordinationMode = strings.TrimSpace(input.CoordinationMode)
	if input.CoordinationMode == "" {
		input.CoordinationMode = "planning"
	}
	if _, ok := allowedCoordinationModes[input.CoordinationMode]; !ok {
		return CreateInput{}, fmt.Errorf("%w: invalid coordination mode", ErrInvalidInput)
	}
	if input.CoordinationMode == "fixed" {
		if input.StartsAt == nil {
			return CreateInput{}, fmt.Errorf("%w: fixed meeting requires starts_at", ErrInvalidInput)
		}
		startsAt, endsAt, err := validateTimeOption(nil, *input.StartsAt, input.EndsAt)
		if err != nil {
			return CreateInput{}, err
		}
		input.StartsAt, input.EndsAt = &startsAt, endsAt
	} else if input.StartsAt != nil || input.EndsAt != nil {
		return CreateInput{}, fmt.Errorf("%w: planning meeting time belongs in time options", ErrInvalidInput)
	}
	return input, nil
}

func validateMetadata(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.EventType = strings.TrimSpace(input.EventType)
	if input.EventType == "" {
		input.EventType = "other"
	}
	if utf8.RuneCountInString(input.Title) < 1 || utf8.RuneCountInString(input.Title) > 120 {
		return CreateInput{}, fmt.Errorf("%w: title must contain 1–120 characters", ErrInvalidInput)
	}
	if utf8.RuneCountInString(input.Description) > 2000 {
		return CreateInput{}, fmt.Errorf("%w: description must contain at most 2000 characters", ErrInvalidInput)
	}
	if _, ok := allowedEventTypes[input.EventType]; !ok {
		return CreateInput{}, fmt.Errorf("%w: invalid event type", ErrInvalidInput)
	}
	coverURL, err := normalizeHTTPSURL(input.CoverURL, "cover URL")
	if err != nil {
		return CreateInput{}, err
	}
	input.CoverURL = coverURL
	locationName, err := normalizeOptionalText(input.LocationName, 200, "location name")
	if err != nil {
		return CreateInput{}, err
	}
	input.LocationName = locationName
	locationURL, err := normalizeHTTPSURL(input.LocationURL, "location URL")
	if err != nil {
		return CreateInput{}, err
	}
	input.LocationURL = locationURL
	return input, nil
}

func normalizeOptionalText(raw *string, maxLength int, field string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(value) > maxLength {
		return nil, fmt.Errorf("%w: %s must contain at most %d characters", ErrInvalidInput, field, maxLength)
	}
	return &value, nil
}

func normalizeHTTPSURL(raw *string, field string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	if len(value) > 2048 {
		return nil, fmt.Errorf("%w: %s must contain at most 2048 bytes", ErrInvalidInput, field)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("%w: %s must be an absolute HTTPS URL without credentials", ErrInvalidInput, field)
	}
	return &value, nil
}

func validateIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 8 || len(value) > 128 {
		return "", fmt.Errorf("%w: Idempotency-Key must contain 8–128 characters", ErrInvalidInput)
	}
	return value, nil
}

func invitationHash(token string) ([]byte, error) {
	token = strings.TrimSpace(token)
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("%w: invitation secret must contain 32 random bytes", ErrInvalidInput)
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
