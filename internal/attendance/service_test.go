package attendance

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type repositoryStub struct {
	status Status
}

func (*repositoryStub) View(context.Context, uuid.UUID, uuid.UUID, int, int) (View, error) {
	return View{ParticipantCount: 1, UnansweredCount: 1, MyStatus: StatusUnanswered}, nil
}

func (r *repositoryStub) SetStatus(
	_ context.Context, _, _ uuid.UUID, status Status,
) (bool, error) {
	r.status = status
	return true, nil
}

func TestRespondAcceptsExplicitAndClearedStatuses(t *testing.T) {
	t.Parallel()
	for _, status := range []Status{StatusGoing, StatusMaybe, StatusNotGoing, StatusUnanswered} {
		status := status
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			repository := &repositoryStub{}
			changed, err := NewService(repository).Respond(
				context.Background(), uuid.New(), uuid.New(), RespondInput{Status: status},
			)
			if err != nil {
				t.Fatalf("Respond() error = %v", err)
			}
			if !changed || repository.status != status {
				t.Fatalf("Respond() changed/status = %v/%q", changed, repository.status)
			}
		})
	}
}

func TestRespondRejectsUnknownStatus(t *testing.T) {
	t.Parallel()
	_, err := NewService(&repositoryStub{}).Respond(
		context.Background(), uuid.New(), uuid.New(), RespondInput{Status: "later"},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Respond() error = %v, want ErrInvalidInput", err)
	}
}
