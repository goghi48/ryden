package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type profileRepositoryStub struct {
	updatedName     string
	updatedNickname string
	updatedAvatar   *string
	updateCalls     int
}

func (r *profileRepositoryStub) CreateUserWithSession(context.Context, NewUser, NewRefreshSession) (User, error) {
	return User{}, nil
}

func (r *profileRepositoryStub) UserByEmail(context.Context, string) (UserWithPassword, error) {
	return UserWithPassword{}, nil
}

func (r *profileRepositoryStub) UserByID(context.Context, uuid.UUID) (User, error) {
	return User{}, nil
}

func (r *profileRepositoryStub) UpdateProfile(
	_ context.Context,
	userID uuid.UUID,
	displayName string,
	nickname string,
	avatarURL *string,
) (User, error) {
	r.updateCalls++
	r.updatedName = displayName
	r.updatedNickname = nickname
	r.updatedAvatar = avatarURL
	return User{
		ID:          userID,
		Email:       "anna@example.test",
		DisplayName: displayName,
		Nickname:    nickname,
		AvatarURL:   avatarURL,
	}, nil
}

func (r *profileRepositoryStub) CreateSession(context.Context, NewRefreshSession) error {
	return nil
}

func (r *profileRepositoryStub) RotateSession(context.Context, []byte, NewRefreshSession) (User, error) {
	return User{}, nil
}

func (r *profileRepositoryStub) RevokeSession(context.Context, []byte) error {
	return nil
}

func TestUpdateProfileNormalizesEditableFields(t *testing.T) {
	repository := &profileRepositoryStub{}
	service := &Service{repository: repository}
	userID := uuid.New()
	avatar := "  https://images.example.test/avatars/anna.png  "

	user, err := service.UpdateProfile(context.Background(), userID, UpdateProfileInput{
		DisplayName: "  Анна Р.  ",
		Nickname:    "  Anna_R  ",
		AvatarURL:   &avatar,
	})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if repository.updateCalls != 1 || repository.updatedName != "Анна Р." || repository.updatedNickname != "anna_r" {
		t.Fatalf("repository update = (%d, %q, %q)", repository.updateCalls, repository.updatedName, repository.updatedNickname)
	}
	if repository.updatedAvatar == nil ||
		*repository.updatedAvatar != "https://images.example.test/avatars/anna.png" {
		t.Fatalf("repository avatar = %#v", repository.updatedAvatar)
	}
	if user.ID != userID || user.DisplayName != repository.updatedName {
		t.Fatalf("UpdateProfile() user = %#v", user)
	}
}

func TestUpdateProfileRejectsUnsafeAvatarURL(t *testing.T) {
	repository := &profileRepositoryStub{}
	service := &Service{repository: repository}
	avatar := "http://images.example.test/anna.png"

	_, err := service.UpdateProfile(context.Background(), uuid.New(), UpdateProfileInput{
		DisplayName: "Анна",
		Nickname:    "anna",
		AvatarURL:   &avatar,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateProfile() error = %v, want ErrInvalidInput", err)
	}
	if repository.updateCalls != 0 {
		t.Fatalf("repository update calls = %d, want 0", repository.updateCalls)
	}
}

func TestUpdateProfileClearsAvatar(t *testing.T) {
	repository := &profileRepositoryStub{}
	service := &Service{repository: repository}

	_, err := service.UpdateProfile(context.Background(), uuid.New(), UpdateProfileInput{
		DisplayName: "Дима",
		Nickname:    "dima",
		AvatarURL:   nil,
	})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if repository.updatedAvatar != nil {
		t.Fatalf("repository avatar = %#v, want nil", repository.updatedAvatar)
	}
}

func TestNormalizeNickname(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		valid bool
	}{
		{name: "lowercases", input: "  Anna_7  ", want: "anna_7", valid: true},
		{name: "too short", input: "ab", valid: false},
		{name: "starts with digit", input: "7anna", valid: false},
		{name: "double underscore", input: "anna__r", valid: false},
		{name: "trailing underscore", input: "anna_", valid: false},
		{name: "cyrillic", input: "анна", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeNickname(test.input)
			if test.valid && (err != nil || got != test.want) {
				t.Fatalf("normalizeNickname() = %q, %v", got, err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("normalizeNickname() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}
