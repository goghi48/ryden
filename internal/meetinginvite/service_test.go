package meetinginvite

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type repositoryStub struct {
	sent []uuid.UUID
}

func (*repositoryStub) Candidates(context.Context, uuid.UUID, uuid.UUID, int, int) (CandidatePage, error) {
	return CandidatePage{}, nil
}

func (*repositoryStub) Incoming(context.Context, uuid.UUID, int, int) (IncomingPage, error) {
	return IncomingPage{}, nil
}

func (r *repositoryStub) Send(_ context.Context, _, _ uuid.UUID, userIDs []uuid.UUID) (int, error) {
	r.sent = append([]uuid.UUID(nil), userIDs...)
	return len(userIDs), nil
}

func (*repositoryStub) Accept(context.Context, uuid.UUID, uuid.UUID) (ResponseMutation, error) {
	return ResponseMutation{Changed: true, Joined: true}, nil
}

func (*repositoryStub) Decline(context.Context, uuid.UUID, uuid.UUID) (ResponseMutation, error) {
	return ResponseMutation{Changed: true}, nil
}

func TestSendDeduplicatesBoundedInvitees(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	ownerID := uuid.New()
	friendID := uuid.New()
	mutation, err := service.Send(
		context.Background(), ownerID, uuid.New(), []uuid.UUID{friendID, friendID},
	)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if mutation.ChangedCount != 1 || len(repository.sent) != 1 || repository.sent[0] != friendID {
		t.Fatalf("Send() = %#v, sent %#v", mutation, repository.sent)
	}
}

func TestSendRejectsOwnerAndOversizedBatch(t *testing.T) {
	service := NewService(&repositoryStub{})
	ownerID := uuid.New()
	if _, err := service.Send(context.Background(), ownerID, uuid.New(), []uuid.UUID{ownerID}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Send(owner) error = %v, want ErrInvalidInput", err)
	}
	batch := make([]uuid.UUID, MaxBatchSize+1)
	for index := range batch {
		batch[index] = uuid.New()
	}
	if _, err := service.Send(context.Background(), ownerID, uuid.New(), batch); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Send(oversized) error = %v, want ErrInvalidInput", err)
	}
}

func TestPaginationIsBounded(t *testing.T) {
	service := NewService(&repositoryStub{})
	if _, err := service.Incoming(context.Background(), uuid.New(), 51, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Incoming() error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.Candidates(context.Background(), uuid.New(), uuid.New(), 50, -1); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Candidates() error = %v, want ErrInvalidInput", err)
	}
}
