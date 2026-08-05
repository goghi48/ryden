package poll

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type repositoryStub struct {
	input          CreateInput
	hashSize       int
	closedOptionID *uuid.UUID
}

func (r *repositoryStub) Create(
	_ context.Context,
	_, _ uuid.UUID,
	_ string,
	hash []byte,
	input CreateInput,
) (Poll, bool, error) {
	r.input = input
	r.hashSize = len(hash)
	return Poll{ID: uuid.New(), Question: input.Question}, false, nil
}

func (*repositoryStub) List(context.Context, uuid.UUID, uuid.UUID) ([]Poll, error) {
	return []Poll{}, nil
}

func (*repositoryStub) History(context.Context, uuid.UUID, uuid.UUID, int, int) (HistoryPage, error) {
	return HistoryPage{Items: []HistoryEntry{}}, nil
}

func (*repositoryStub) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (*repositoryStub) Vote(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (bool, error) {
	return true, nil
}

func (r *repositoryStub) Close(
	_ context.Context,
	_, _, _ uuid.UUID,
	selectedOptionID *uuid.UUID,
) (bool, error) {
	r.closedOptionID = selectedOptionID
	return true, nil
}

func TestCreateNormalizesPollAndHashesRequest(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	result, replayed, err := service.Create(
		context.Background(), uuid.New(), uuid.New(), "poll-key-123",
		CreateInput{
			Question:     "  Что взять с собой? ",
			ResponseMode: "multiple",
			IsAnonymous:  true,
			AllowRevote:  false,
			Options:      []string{"  Вода", "Плед  "},
		},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if replayed || result.Question != "Что взять с собой?" {
		t.Fatalf("Create() = (%#v, %v)", result, replayed)
	}
	if repository.hashSize != 32 || repository.input.Options[0] != "Вода" || !repository.input.IsAnonymous || repository.input.AllowRevote {
		t.Fatalf("normalized input = %#v, hash size = %d", repository.input, repository.hashSize)
	}
}

func TestCreateRejectsDuplicateOptions(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, _, err := service.Create(
		context.Background(), uuid.New(), uuid.New(), "poll-key-123",
		CreateInput{
			Question: "Напиток?", ResponseMode: "single",
			Options: []string{"Чай", " чай "},
		},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}

func TestVoteRejectsDuplicateOptionIDs(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	optionID := uuid.New()
	_, err := service.Vote(
		context.Background(), uuid.New(), uuid.New(),
		VoteInput{OptionIDs: []uuid.UUID{optionID, optionID}},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Vote() error = %v, want ErrInvalidInput", err)
	}
}

func TestHistoryValidatesPagination(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	if _, err := service.History(
		context.Background(), uuid.New(), uuid.New(), 0, 0,
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("History() error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.History(
		context.Background(), uuid.New(), uuid.New(), 50, -1,
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("History() error = %v, want ErrInvalidInput", err)
	}
}

func TestVoteHistoryHelpersNormalizeEffectiveChanges(t *testing.T) {
	t.Parallel()

	first := uuid.New()
	second := uuid.New()
	if !sameUUIDSlice([]uuid.UUID{first, second}, []uuid.UUID{first, second}) {
		t.Fatal("sameUUIDSlice() did not recognize the same normalized set")
	}
	if sameUUIDSlice([]uuid.UUID{first, second}, []uuid.UUID{second, first}) {
		t.Fatal("sameUUIDSlice() accepted a non-normalized order")
	}
	if got := voteHistoryAction(nil, []uuid.UUID{first}); got != "cast" {
		t.Fatalf("voteHistoryAction(cast) = %q", got)
	}
	if got := voteHistoryAction([]uuid.UUID{first}, []uuid.UUID{second}); got != "change" {
		t.Fatalf("voteHistoryAction(change) = %q", got)
	}
	if got := voteHistoryAction([]uuid.UUID{first}, nil); got != "retract" {
		t.Fatalf("voteHistoryAction(retract) = %q", got)
	}
}

func TestCloseAllowsStoppingWithoutSelectedOption(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	changed, err := service.Close(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		CloseInput{SelectedOptionID: nil},
	)
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !changed {
		t.Fatal("Close() changed = false, want true")
	}
	if repository.closedOptionID != nil {
		t.Fatalf("Close() selected option = %v, want nil", repository.closedOptionID)
	}
}

func TestCloseRejectsNilUUIDPointer(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	invalid := uuid.Nil
	_, err := service.Close(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		CloseInput{SelectedOptionID: &invalid},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Close() error = %v, want ErrInvalidInput", err)
	}
}
