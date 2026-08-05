package friendship

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type repositoryStub struct {
	searchPrefix string
	sendChanged  bool
	sendErr      error
}

func (r *repositoryStub) Search(_ context.Context, _ uuid.UUID, prefix string, _ int) ([]Person, error) {
	r.searchPrefix = prefix
	return []Person{}, nil
}
func (*repositoryStub) Overview(context.Context, uuid.UUID, int, int) (Overview, error) {
	return Overview{}, nil
}
func (r *repositoryStub) Send(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return r.sendChanged, r.sendErr
}
func (*repositoryStub) Accept(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
func (*repositoryStub) DeleteRequest(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}
func (*repositoryStub) RemoveFriend(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

func TestSearchNormalizesNicknamePrefix(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	if _, err := service.Search(context.Background(), uuid.New(), "  Anna_1 ", 20); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if repository.searchPrefix != "anna_1" {
		t.Fatalf("Search() prefix = %q", repository.searchPrefix)
	}
}

func TestSearchRejectsWildcardCharacters(t *testing.T) {
	service := NewService(&repositoryStub{})
	for _, prefix := range []string{"an%", "an?", "анна", "a__b"} {
		if _, err := service.Search(context.Background(), uuid.New(), prefix, 20); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Search(%q) error = %v, want ErrInvalidInput", prefix, err)
		}
	}
}

func TestSendRejectsSelf(t *testing.T) {
	service := NewService(&repositoryStub{})
	userID := uuid.New()
	if _, err := service.Send(context.Background(), userID, userID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Send() error = %v, want ErrInvalidInput", err)
	}
}

func TestSendReturnsMutationState(t *testing.T) {
	service := NewService(&repositoryStub{sendChanged: true})
	result, err := service.Send(context.Background(), uuid.New(), uuid.New())
	if err != nil || !result.Changed {
		t.Fatalf("Send() = %#v, %v", result, err)
	}
}
