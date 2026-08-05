package note

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type repositoryStub struct {
	text string
}

func (*repositoryStub) List(context.Context, uuid.UUID, uuid.UUID, int, int) (Page, error) {
	return Page{}, nil
}

func (r *repositoryStub) Upsert(_ context.Context, _, _ uuid.UUID, text string) (bool, error) {
	r.text = text
	return true, nil
}

func (*repositoryStub) Delete(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

func TestUpsertTrimsAndAcceptsBoundedText(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{}
	changed, err := NewService(repository).Upsert(
		context.Background(), uuid.New(), uuid.New(), UpsertInput{Text: "  Bring cards  "},
	)
	if err != nil || !changed || repository.text != "Bring cards" {
		t.Fatalf("Upsert() = (%v, %v), stored %q", changed, err, repository.text)
	}
}

func TestUpsertRejectsEmptyAndOversizedText(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"   ", strings.Repeat("я", 201)} {
		_, err := NewService(&repositoryStub{}).Upsert(
			context.Background(), uuid.New(), uuid.New(), UpsertInput{Text: text},
		)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Upsert(%d runes) error = %v, want ErrInvalidInput", len([]rune(text)), err)
		}
	}
}

func TestListRejectsUnboundedPagination(t *testing.T) {
	t.Parallel()
	_, err := NewService(&repositoryStub{}).List(context.Background(), uuid.New(), uuid.New(), 101, 0)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("List() error = %v, want ErrInvalidInput", err)
	}
}
