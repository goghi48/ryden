package decision

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCompatibleAcceptsGeneralOrMatchingPlanTime(t *testing.T) {
	t.Parallel()
	planID := uuid.New()
	otherPlanID := uuid.New()
	if !compatible(planID, nil) {
		t.Fatal("general time must be compatible")
	}
	if !compatible(planID, &planID) {
		t.Fatal("time scoped to selected plan must be compatible")
	}
	if compatible(planID, &otherPlanID) {
		t.Fatal("time scoped to another plan must be incompatible")
	}
}

func TestSameDecisionRequiresBothMatchingIDs(t *testing.T) {
	t.Parallel()
	planID := uuid.New()
	timeID := uuid.New()
	if !sameDecision(&planID, &timeID, planID, timeID) {
		t.Fatal("matching final decision must be detected")
	}
	if sameDecision(nil, &timeID, planID, timeID) {
		t.Fatal("incomplete stored decision must not match")
	}
}

func TestServiceValidatesPaginationAndFinalDecision(t *testing.T) {
	t.Parallel()
	repository := &stubRepository{}
	service := NewService(repository)
	if _, err := service.List(context.Background(), uuid.New(), uuid.New(), 101, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("List error = %v, want ErrInvalidInput", err)
	}
	if _, _, err := service.Finalize(
		context.Background(),
		uuid.New(),
		uuid.New(),
		FinalizeInput{},
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Finalize error = %v, want ErrInvalidInput", err)
	}
	if repository.called {
		t.Fatal("repository must not be called for invalid input")
	}
}

type stubRepository struct {
	called bool
}

func (r *stubRepository) List(context.Context, uuid.UUID, uuid.UUID, int, int) (Page, error) {
	r.called = true
	return Page{}, nil
}

func (r *stubRepository) Vote(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (bool, error) {
	r.called = true
	return false, nil
}

func (r *stubRepository) Finalize(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
) (FinalDecision, bool, error) {
	r.called = true
	return FinalDecision{}, false, nil
}
