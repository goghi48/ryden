package availability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildViewRanksGeneralAndScopedTimesPerPlan(t *testing.T) {
	t.Parallel()
	ownerID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	friendID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	planA := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	planB := uuid.MustParse("10000000-0000-0000-0000-000000000002")
	general := uuid.MustParse("20000000-0000-0000-0000-000000000001")
	scopedA := uuid.MustParse("20000000-0000-0000-0000-000000000002")
	scopedB := uuid.MustParse("20000000-0000-0000-0000-000000000003")
	start := time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)

	view := buildView(ownerID, Snapshot{
		Plans: []PlanOption{{ID: planA, Title: "Пикник"}, {ID: planB, Title: "Кафе"}},
		Times: []TimeOption{
			{ID: general, StartsAt: start, EndsAt: timePointer(start.Add(2 * time.Hour)), Position: 0},
			{ID: scopedA, PlanOptionID: &planA, StartsAt: start.Add(time.Hour), EndsAt: timePointer(start.Add(3 * time.Hour)), Position: 1},
			{ID: scopedB, PlanOptionID: &planB, StartsAt: start.Add(2 * time.Hour), EndsAt: timePointer(start.Add(4 * time.Hour)), Position: 2},
		},
		Participants: []Participant{{UserID: ownerID}, {UserID: friendID}},
		Votes: []Vote{
			{TimeOptionID: general, UserID: ownerID, Status: StatusAvailable},
			{TimeOptionID: general, UserID: friendID, Status: StatusAvailable},
			{TimeOptionID: scopedA, UserID: ownerID, Status: StatusPreferred},
			{TimeOptionID: scopedA, UserID: friendID, Status: StatusUnavailable},
			{TimeOptionID: scopedB, UserID: ownerID, Status: StatusPreferred},
			{TimeOptionID: scopedB, UserID: friendID, Status: StatusPreferred},
		},
	})

	if got := view.Recommendations[0].TimeOptionID; got != general {
		t.Fatalf("plan A recommendation = %s, want general %s", got, general)
	}
	if got := view.Recommendations[1].TimeOptionID; got != scopedB {
		t.Fatalf("plan B recommendation = %s, want scoped %s", got, scopedB)
	}
	if view.Recommendations[0].Provisional || view.Recommendations[1].Provisional {
		t.Fatal("fully answered recommendations must not be provisional")
	}
	if view.Items[0].Score != 4 {
		t.Fatalf("general score = %d, want 4", view.Items[0].Score)
	}
}

func TestBuildViewUsesDeterministicEarlierTieBreakAndMarksIncomplete(t *testing.T) {
	t.Parallel()
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	otherID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	planID := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	earlyID := uuid.MustParse("20000000-0000-0000-0000-000000000001")
	lateID := uuid.MustParse("20000000-0000-0000-0000-000000000002")
	start := time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)

	view := buildView(userID, Snapshot{
		Plans: []PlanOption{{ID: planID, Title: "Прогулка"}},
		Times: []TimeOption{
			{ID: lateID, StartsAt: start.Add(time.Hour), EndsAt: timePointer(start.Add(2 * time.Hour)), Position: 0},
			{ID: earlyID, StartsAt: start, EndsAt: timePointer(start.Add(time.Hour)), Position: 1},
		},
		Participants: []Participant{{UserID: userID}, {UserID: otherID}},
		Votes: []Vote{
			{TimeOptionID: lateID, UserID: userID, Status: StatusAvailable},
			{TimeOptionID: earlyID, UserID: userID, Status: StatusAvailable},
		},
	})

	if got := view.Recommendations[0].TimeOptionID; got != earlyID {
		t.Fatalf("recommendation = %s, want earlier %s", got, earlyID)
	}
	if !view.Recommendations[0].Provisional {
		t.Fatal("recommendation with an unanswered participant must be provisional")
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func TestRespondRejectsUnknownStatusBeforeRepository(t *testing.T) {
	t.Parallel()
	repository := &stubRepository{}
	service := NewService(repository)
	_, err := service.Respond(
		context.Background(), uuid.New(), uuid.New(), RespondInput{Status: "sometimes"},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Respond error = %v, want ErrInvalidInput", err)
	}
	if repository.called {
		t.Fatal("repository must not be called for an invalid status")
	}
}

type stubRepository struct {
	called bool
}

func (r *stubRepository) Snapshot(context.Context, uuid.UUID, uuid.UUID) (Snapshot, error) {
	r.called = true
	return Snapshot{}, nil
}

func (r *stubRepository) SetStatus(context.Context, uuid.UUID, uuid.UUID, Status) (bool, error) {
	r.called = true
	return false, nil
}
