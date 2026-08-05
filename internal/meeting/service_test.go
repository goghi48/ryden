package meeting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type repositoryStub struct {
	input           CreateInput
	idempotencyKey  string
	requestHashSize int
	updateInput     UpdateInput
	updateResult    Meeting
	updateErr       error
	updatePlanInput UpdatePlanOptionInput
	addTimeInput    AddTimeOptionInput
	updateTimeInput UpdateTimeOptionInput
}

func (r *repositoryStub) Create(
	_ context.Context,
	_ uuid.UUID,
	key string,
	hash []byte,
	input CreateInput,
) (Meeting, bool, error) {
	r.input = input
	r.idempotencyKey = key
	r.requestHashSize = len(hash)
	return Meeting{ID: uuid.New(), Title: input.Title}, false, nil
}

func (r *repositoryStub) Update(
	_ context.Context,
	_, _ uuid.UUID,
	input UpdateInput,
) (Meeting, error) {
	r.updateInput = input
	return r.updateResult, r.updateErr
}

func (*repositoryStub) List(context.Context, uuid.UUID, int, int) ([]Meeting, error) {
	return []Meeting{}, nil
}

func (*repositoryStub) Get(context.Context, uuid.UUID, uuid.UUID) (Detail, error) {
	return Detail{}, ErrNotFound
}

func (*repositoryStub) AddPlanOption(
	context.Context, uuid.UUID, uuid.UUID, string, AddPlanOptionInput,
) (PlanOption, bool, error) {
	return PlanOption{}, false, nil
}

func (r *repositoryStub) UpdatePlanOption(
	_ context.Context, _, _, _ uuid.UUID, input UpdatePlanOptionInput,
) (PlanOption, error) {
	r.updatePlanInput = input
	return PlanOption{Title: input.Title, Description: input.Description}, nil
}

func (*repositoryStub) DeletePlanOption(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (r *repositoryStub) AddTimeOption(
	_ context.Context, _, _ uuid.UUID, _ string, input AddTimeOptionInput,
) (TimeOption, bool, error) {
	r.addTimeInput = input
	return TimeOption{}, false, nil
}

func (r *repositoryStub) UpdateTimeOption(
	_ context.Context, _, _, _ uuid.UUID, input UpdateTimeOptionInput,
) (TimeOption, error) {
	r.updateTimeInput = input
	return TimeOption{PlanOptionID: input.PlanOptionID, StartsAt: input.StartsAt, EndsAt: input.EndsAt}, nil
}

func (*repositoryStub) DeleteTimeOption(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (*repositoryStub) CreateInvitation(
	context.Context, uuid.UUID, uuid.UUID, string, []byte,
) (Invitation, bool, error) {
	return Invitation{}, false, nil
}

func (*repositoryStub) RevokeInvitation(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

func (*repositoryStub) JoinInvitation(context.Context, uuid.UUID, []byte) (Detail, bool, error) {
	return Detail{}, false, ErrInvitationInvalid
}

func (*repositoryStub) Complete(
	context.Context, uuid.UUID, uuid.UUID,
) (Completion, bool, error) {
	return Completion{State: "completed"}, false, nil
}

func (*repositoryStub) Cancel(
	context.Context, uuid.UUID, uuid.UUID,
) (Cancellation, bool, error) {
	return Cancellation{State: "cancelled"}, false, nil
}

func TestCreateNormalizesAndHashesRequest(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	coverURL := "  https://images.example.test/meetings/games.jpg  "
	locationName := "  Дом Анны  "
	locationURL := "  https://maps.example.test/place/anna  "

	result, replayed, err := service.Create(context.Background(), uuid.New(), " request-key-123 ", CreateInput{
		Title: "  Настольные игры  ", Description: "  В субботу  ", EventType: "game_night",
		CoverURL: &coverURL, LocationName: &locationName, LocationURL: &locationURL,
		Timezone: "Asia/Novosibirsk",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if replayed {
		t.Fatal("Create() replayed = true, want false")
	}
	if result.Title != "Настольные игры" {
		t.Fatalf("result.Title = %q", result.Title)
	}
	if repository.input.EventType != "game_night" {
		t.Fatalf("EventType = %q, want game_night", repository.input.EventType)
	}
	if repository.input.CoordinationMode != "planning" {
		t.Fatalf("CoordinationMode = %q, want planning", repository.input.CoordinationMode)
	}
	if repository.input.CoverURL == nil ||
		*repository.input.CoverURL != "https://images.example.test/meetings/games.jpg" {
		t.Fatalf("CoverURL = %#v", repository.input.CoverURL)
	}
	if repository.input.LocationName == nil || *repository.input.LocationName != "Дом Анны" {
		t.Fatalf("LocationName = %#v", repository.input.LocationName)
	}
	if repository.input.LocationURL == nil ||
		*repository.input.LocationURL != "https://maps.example.test/place/anna" {
		t.Fatalf("LocationURL = %#v", repository.input.LocationURL)
	}
	if repository.idempotencyKey != "request-key-123" {
		t.Fatalf("idempotency key = %q", repository.idempotencyKey)
	}
	if repository.requestHashSize != 32 {
		t.Fatalf("request hash size = %d, want 32", repository.requestHashSize)
	}
}

func TestCreateAcceptsFixedCoordinationMode(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	startsAt := time.Date(2026, time.August, 8, 11, 0, 0, 0, time.FixedZone("NOVT", 7*60*60))
	endsAt := startsAt.Add(3 * time.Hour)
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Ужин у Анны", CoordinationMode: "fixed", Timezone: "Asia/Novosibirsk",
		StartsAt: &startsAt, EndsAt: &endsAt,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.input.CoordinationMode != "fixed" {
		t.Fatalf("CoordinationMode = %q, want fixed", repository.input.CoordinationMode)
	}
	if repository.input.StartsAt == nil || !repository.input.StartsAt.Equal(startsAt.UTC()) ||
		repository.input.EndsAt == nil || !repository.input.EndsAt.Equal(endsAt.UTC()) {
		t.Fatalf("fixed time = (%v, %v), want (%v, %v)",
			repository.input.StartsAt, repository.input.EndsAt, startsAt.UTC(), endsAt.UTC())
	}
}

func TestCreateAcceptsFixedMeetingWithoutDuration(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	startsAt := time.Date(2026, time.August, 8, 11, 5, 0, 0, time.FixedZone("NOVT", 7*60*60))
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Ужин у Анны", CoordinationMode: "fixed", Timezone: "Asia/Novosibirsk",
		StartsAt: &startsAt,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.input.EndsAt != nil {
		t.Fatalf("EndsAt = %v, want nil", repository.input.EndsAt)
	}
}

func TestCreateRejectsFixedMeetingWithoutTime(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Ужин у Анны", CoordinationMode: "fixed", Timezone: "Asia/Novosibirsk",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRejectsUnknownCoordinationMode(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Встреча", CoordinationMode: "public_event", Timezone: "Asia/Novosibirsk",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRejectsUnsafeCoverURL(t *testing.T) {
	t.Parallel()

	coverURL := "http://images.example.test/meeting.jpg"
	service := NewService(&repositoryStub{})
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Встреча", CoverURL: &coverURL, Timezone: "Asia/Novosibirsk",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRejectsUnsafeLocationURL(t *testing.T) {
	t.Parallel()

	locationURL := "http://maps.example.test/place"
	service := NewService(&repositoryStub{})
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Встреча", LocationURL: &locationURL, Timezone: "Asia/Novosibirsk",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateAcceptsEmptyLocationURL(t *testing.T) {
	t.Parallel()

	locationURL := "   "
	repository := &repositoryStub{}
	service := NewService(repository)
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Встреча", LocationURL: &locationURL, Timezone: "Asia/Novosibirsk",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.input.LocationURL != nil {
		t.Fatalf("LocationURL = %#v, want nil", repository.input.LocationURL)
	}
}

func TestCreateRejectsUnknownEventType(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Встреча", EventType: "conference", Timezone: "Asia/Novosibirsk",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRejectsUnknownTimezone(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, _, err := service.Create(context.Background(), uuid.New(), "request-key-123", CreateInput{
		Title: "Встреча", Timezone: "Mars/Olympus",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateNormalizesMetadataAndPreservesExpectedVersion(t *testing.T) {
	t.Parallel()

	coverURL := "  https://images.example.test/updated.jpg  "
	locationName := "  Дом Анны  "
	locationURL := "  https://maps.example.test/anna  "
	repository := &repositoryStub{updateResult: Meeting{ID: uuid.New(), Version: 8}}
	service := NewService(repository)

	result, err := service.Update(context.Background(), uuid.New(), uuid.New(), UpdateInput{
		Title: "  Игровой вечер  ", Description: "  Берём любимые игры  ", EventType: "game_night",
		CoverURL: &coverURL, LocationName: &locationName, LocationURL: &locationURL,
		ExpectedVersion: 7,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Version != 8 {
		t.Fatalf("result.Version = %d, want 8", result.Version)
	}
	if repository.updateInput.Title != "Игровой вечер" ||
		repository.updateInput.Description != "Берём любимые игры" {
		t.Fatalf("normalized input = %#v", repository.updateInput)
	}
	if repository.updateInput.CoverURL == nil ||
		*repository.updateInput.CoverURL != "https://images.example.test/updated.jpg" {
		t.Fatalf("CoverURL = %#v", repository.updateInput.CoverURL)
	}
	if repository.updateInput.LocationName == nil || *repository.updateInput.LocationName != "Дом Анны" {
		t.Fatalf("LocationName = %#v", repository.updateInput.LocationName)
	}
	if repository.updateInput.LocationURL == nil ||
		*repository.updateInput.LocationURL != "https://maps.example.test/anna" {
		t.Fatalf("LocationURL = %#v", repository.updateInput.LocationURL)
	}
	if repository.updateInput.ExpectedVersion != 7 {
		t.Fatalf("ExpectedVersion = %d, want 7", repository.updateInput.ExpectedVersion)
	}
}

func TestUpdateRejectsInvalidVersion(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, err := service.Update(context.Background(), uuid.New(), uuid.New(), UpdateInput{
		Title: "Встреча", EventType: "other", ExpectedVersion: 0,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateRejectsUnsafeLocationURL(t *testing.T) {
	t.Parallel()

	locationURL := "http://maps.example.test/anna"
	service := NewService(&repositoryStub{})
	_, err := service.Update(context.Background(), uuid.New(), uuid.New(), UpdateInput{
		Title: "Встреча", EventType: "other", LocationURL: &locationURL, ExpectedVersion: 1,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateAcceptsEmptyLocationURL(t *testing.T) {
	t.Parallel()

	locationURL := ""
	repository := &repositoryStub{updateResult: Meeting{ID: uuid.New(), Version: 2}}
	service := NewService(repository)
	_, err := service.Update(context.Background(), uuid.New(), uuid.New(), UpdateInput{
		Title: "Встреча", EventType: "other", LocationURL: &locationURL, ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.updateInput.LocationURL != nil {
		t.Fatalf("LocationURL = %#v, want nil", repository.updateInput.LocationURL)
	}
}

func TestUpdateNormalizesOptionalFixedTime(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{updateResult: Meeting{ID: uuid.New(), Version: 8}}
	service := NewService(repository)
	location := time.FixedZone("meeting", 7*60*60)
	start := time.Date(2026, time.August, 18, 19, 5, 0, 0, location)
	end := start.Add(2*time.Hour + 35*time.Minute)
	_, err := service.Update(context.Background(), uuid.New(), uuid.New(), UpdateInput{
		Title: "Новый ужин", EventType: "other", StartsAt: &start, EndsAt: &end, ExpectedVersion: 7,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.updateInput.StartsAt == nil || !repository.updateInput.StartsAt.Equal(start.UTC()) {
		t.Fatalf("StartsAt = %v, want %v", repository.updateInput.StartsAt, start.UTC())
	}
	if repository.updateInput.EndsAt == nil || !repository.updateInput.EndsAt.Equal(end.UTC()) {
		t.Fatalf("EndsAt = %v, want %v", repository.updateInput.EndsAt, end.UTC())
	}
}

func TestUpdateRejectsEndWithoutStart(t *testing.T) {
	t.Parallel()

	end := time.Now().UTC().Add(time.Hour)
	service := NewService(&repositoryStub{})
	_, err := service.Update(context.Background(), uuid.New(), uuid.New(), UpdateInput{
		Title: "Ужин", EventType: "other", EndsAt: &end, ExpectedVersion: 1,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
	}
}

func TestListRejectsUnboundedPagination(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, err := service.List(context.Background(), uuid.New(), 101, 0)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("List() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateInvitationRejectsWeakSecret(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, _, err := service.CreateInvitation(
		context.Background(), uuid.New(), uuid.New(), "invitation-key-1",
		CreateInvitationInput{Secret: "predictable"},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateInvitation() error = %v, want ErrInvalidInput", err)
	}
}

func TestAddTimeOptionRequiresPositiveRange(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	start := time.Now()
	_, _, err := service.AddTimeOption(
		context.Background(), uuid.New(), uuid.New(), "time-key-123",
		AddTimeOptionInput{StartsAt: start, EndsAt: &start},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("AddTimeOption() error = %v, want ErrInvalidInput", err)
	}
}

func TestAddTimeOptionRequiresFiveMinuteStart(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	start := time.Date(2026, time.August, 8, 11, 3, 0, 0, time.UTC)
	_, _, err := service.AddTimeOption(
		context.Background(), uuid.New(), uuid.New(), "time-key-123",
		AddTimeOptionInput{StartsAt: start},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("AddTimeOption() error = %v, want ErrInvalidInput", err)
	}
}

func TestAddTimeOptionAcceptsOptionalDuration(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	start := time.Date(2026, time.August, 8, 11, 5, 0, 0, time.UTC)
	_, _, err := service.AddTimeOption(
		context.Background(), uuid.New(), uuid.New(), "time-key-123",
		AddTimeOptionInput{StartsAt: start},
	)
	if err != nil {
		t.Fatalf("AddTimeOption() error = %v", err)
	}
	if repository.addTimeInput.EndsAt != nil {
		t.Fatalf("EndsAt = %v, want nil", repository.addTimeInput.EndsAt)
	}
}

func TestAddTimeOptionAcceptsArbitraryMinuteDurationWithinThirtyDays(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	start := time.Date(2026, time.August, 8, 11, 5, 0, 0, time.UTC)
	oneMinute := start.Add(time.Minute)
	if _, _, err := service.AddTimeOption(
		context.Background(), uuid.New(), uuid.New(), "time-key-one",
		AddTimeOptionInput{StartsAt: start, EndsAt: &oneMinute},
	); err != nil {
		t.Fatalf("AddTimeOption(1 minute) error = %v", err)
	}
	maximum := start.Add(30*24*time.Hour + 23*time.Hour + 59*time.Minute)
	if _, _, err := service.AddTimeOption(
		context.Background(), uuid.New(), uuid.New(), "time-key-maximum",
		AddTimeOptionInput{StartsAt: start, EndsAt: &maximum},
	); err != nil {
		t.Fatalf("AddTimeOption(maximum) error = %v", err)
	}
	tooLong := start.Add(31 * 24 * time.Hour)
	if _, _, err := service.AddTimeOption(
		context.Background(), uuid.New(), uuid.New(), "time-key-too-long",
		AddTimeOptionInput{StartsAt: start, EndsAt: &tooLong},
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("AddTimeOption(31 days) error = %v, want ErrInvalidInput", err)
	}
}

func TestUpdatePlanOptionNormalizesContentAndVersion(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	result, err := service.UpdatePlanOption(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		UpdatePlanOptionInput{
			Title: "  Ужин на веранде  ", Description: "  Если будет тепло  ",
			ExpectedVersion: 4,
		},
	)
	if err != nil {
		t.Fatalf("UpdatePlanOption() error = %v", err)
	}
	if result.Title != "Ужин на веранде" || result.Description != "Если будет тепло" {
		t.Fatalf("UpdatePlanOption() = %#v", result)
	}
	if repository.updatePlanInput.ExpectedVersion != 4 {
		t.Fatalf("ExpectedVersion = %d, want 4", repository.updatePlanInput.ExpectedVersion)
	}
}

func TestUpdateTimeOptionNormalizesUTCAndRequiresVersion(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	location := time.FixedZone("local", 7*60*60)
	start := time.Date(2026, time.August, 8, 18, 0, 0, 0, location)
	end := start.Add(2 * time.Hour)
	result, err := service.UpdateTimeOption(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		UpdateTimeOptionInput{StartsAt: start, EndsAt: &end, ExpectedVersion: 5},
	)
	if err != nil {
		t.Fatalf("UpdateTimeOption() error = %v", err)
	}
	if result.StartsAt.Location() != time.UTC || !result.StartsAt.Equal(start) {
		t.Fatalf("StartsAt = %v, want UTC %v", result.StartsAt, start.UTC())
	}
	if repository.updateTimeInput.ExpectedVersion != 5 {
		t.Fatalf("ExpectedVersion = %d, want 5", repository.updateTimeInput.ExpectedVersion)
	}

	_, err = service.UpdateTimeOption(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		UpdateTimeOptionInput{StartsAt: start, EndsAt: &end, ExpectedVersion: 0},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateTimeOption(version 0) error = %v, want ErrInvalidInput", err)
	}
}
