package preparation

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakeRepository struct {
	createInput CreateInput
	updateInput UpdateInput
	deleted     bool
	claim       int32
	status      string
}

func (f *fakeRepository) Update(
	_ context.Context,
	_, _, _ uuid.UUID,
	input UpdateInput,
) (bool, error) {
	f.updateInput = input
	return true, nil
}

func (f *fakeRepository) Delete(
	_ context.Context,
	_, _, _ uuid.UUID,
) (bool, error) {
	f.deleted = true
	return true, nil
}

func (f *fakeRepository) List(
	context.Context, uuid.UUID, uuid.UUID, int, int,
) (Page, error) {
	return Page{}, nil
}

func (f *fakeRepository) Create(
	_ context.Context,
	_, _ uuid.UUID,
	_ string,
	_ []byte,
	input CreateInput,
) (Requirement, bool, error) {
	f.createInput = input
	return Requirement{Name: input.Name, RequiredQuantity: input.RequiredQuantity}, false, nil
}

func (f *fakeRepository) SetClaim(
	_ context.Context,
	_, _, _ uuid.UUID,
	quantity int32,
) (bool, error) {
	f.claim = quantity
	return true, nil
}

func (f *fakeRepository) SetStatus(
	_ context.Context,
	_, _, _ uuid.UUID,
	status string,
) (bool, error) {
	f.status = status
	return true, nil
}

func TestCreateValidatesAndNormalizes(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	result, replayed, err := service.Create(
		context.Background(),
		uuid.New(),
		uuid.New(),
		"requirement-key",
		CreateInput{Name: "  Water bottles  ", RequiredQuantity: 10},
	)
	if err != nil || replayed {
		t.Fatalf("Create() = (%#v, %v, %v)", result, replayed, err)
	}
	if repository.createInput.Name != "Water bottles" ||
		repository.createInput.RequiredQuantity != 10 {
		t.Fatalf("normalized input = %#v", repository.createInput)
	}
}

func TestCreateRejectsInvalidValues(t *testing.T) {
	service := NewService(&fakeRepository{})
	cases := []CreateInput{
		{Name: "", RequiredQuantity: 1},
		{Name: "Water", RequiredQuantity: 0},
		{Name: "Water", RequiredQuantity: 100_001},
	}
	for _, input := range cases {
		if _, _, err := service.Create(
			context.Background(), uuid.New(), uuid.New(), "requirement-key", input,
		); err == nil {
			t.Fatalf("Create(%#v) returned nil error", input)
		}
	}
}

func TestSetClaimAcceptsRetractionAndRejectsNegative(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	if _, err := service.SetClaim(
		context.Background(), uuid.New(), uuid.New(), uuid.New(), ClaimInput{Quantity: 0},
	); err != nil {
		t.Fatalf("SetClaim(retract) error = %v", err)
	}
	if repository.claim != 0 {
		t.Fatalf("stored claim = %d, want 0", repository.claim)
	}
	if _, err := service.SetClaim(
		context.Background(), uuid.New(), uuid.New(), uuid.New(), ClaimInput{Quantity: -1},
	); err == nil {
		t.Fatal("SetClaim(negative) returned nil error")
	}
}

func TestUpdateValidatesAndNormalizes(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	if _, err := service.Update(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		UpdateInput{Name: "  Picnic blankets  ", RequiredQuantity: 3},
	); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.updateInput.Name != "Picnic blankets" ||
		repository.updateInput.RequiredQuantity != 3 {
		t.Fatalf("normalized update = %#v", repository.updateInput)
	}
	for _, input := range []UpdateInput{
		{Name: "", RequiredQuantity: 1},
		{Name: "Blankets", RequiredQuantity: 0},
	} {
		if _, err := service.Update(
			context.Background(), uuid.New(), uuid.New(), uuid.New(), input,
		); err == nil {
			t.Fatalf("Update(%#v) returned nil error", input)
		}
	}
}

func TestDeleteRejectsMissingRequirementID(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	if _, err := service.Delete(
		context.Background(), uuid.New(), uuid.New(), uuid.Nil,
	); err == nil {
		t.Fatal("Delete(nil id) returned nil error")
	}
	if repository.deleted {
		t.Fatal("repository Delete() was called for invalid id")
	}
}

func TestSetStatusValidatesStatus(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	if _, err := service.SetStatus(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		StatusInput{Status: StatusCompleted},
	); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if repository.status != StatusCompleted {
		t.Fatalf("stored status = %q", repository.status)
	}
	if _, err := service.SetStatus(
		context.Background(), uuid.New(), uuid.New(), uuid.New(),
		StatusInput{Status: "blocked"},
	); err == nil {
		t.Fatal("SetStatus(invalid) returned nil error")
	}
}
