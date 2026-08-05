package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrEmailTaken       = errors.New("email already registered")
	ErrNicknameTaken    = errors.New("nickname already registered")
	ErrInvalidLogin     = errors.New("invalid email or password")
	ErrUnauthorized     = errors.New("authentication required")
	ErrSessionNotActive = errors.New("refresh session is not active")
)

type User struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	DisplayName    string    `json:"display_name"`
	Nickname       string    `json:"nickname"`
	AvatarURL      *string   `json:"avatar_url"`
	AvatarRevision *int64    `json:"avatar_revision"`
}

type Session struct {
	User             User      `json:"user"`
	AccessToken      string    `json:"access_token"`
	AccessTokenUntil time.Time `json:"access_token_expires_at"`
	RefreshToken     string    `json:"-"`
	RefreshUntil     time.Time `json:"-"`
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	Nickname    string
}

type LoginInput struct {
	Email    string
	Password string
}

type UpdateProfileInput struct {
	DisplayName string
	Nickname    string
	AvatarURL   *string
}

type Repository interface {
	CreateUserWithSession(context.Context, NewUser, NewRefreshSession) (User, error)
	UserByEmail(context.Context, string) (UserWithPassword, error)
	UserByID(context.Context, uuid.UUID) (User, error)
	UpdateProfile(context.Context, uuid.UUID, string, string, *string) (User, error)
	CreateSession(context.Context, NewRefreshSession) error
	RotateSession(context.Context, []byte, NewRefreshSession) (User, error)
	RevokeSession(context.Context, []byte) error
}

type NewUser struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	DisplayName  string
	Nickname     string
}

type UserWithPassword struct {
	User
	PasswordHash string
}

type NewRefreshSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	ExpiresAt time.Time
}

type Service struct {
	repository        Repository
	tokens            *TokenManager
	refreshTTL        time.Duration
	dummyPasswordHash string
	now               func() time.Time
}

func NewService(repository Repository, tokens *TokenManager, refreshTTL time.Duration) (*Service, error) {
	dummyPasswordHash, err := HashPassword("not-a-real-user-password")
	if err != nil {
		return nil, fmt.Errorf("prepare login timing protection: %w", err)
	}
	return &Service{
		repository:        repository,
		tokens:            tokens,
		refreshTTL:        refreshTTL,
		dummyPasswordHash: dummyPasswordHash,
		now:               time.Now,
	}, nil
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (Session, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return Session{}, err
	}
	name, err := normalizeDisplayName(input.DisplayName)
	if err != nil {
		return Session{}, err
	}
	nickname, err := normalizeNickname(input.Nickname)
	if err != nil {
		return Session{}, err
	}
	if len(input.Password) < 10 || len(input.Password) > 128 {
		return Session{}, fmt.Errorf("%w: password must contain 10–128 characters", ErrInvalidInput)
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return Session{}, err
	}

	userID := uuid.New()
	refresh, newSession, err := s.newRefreshSession(userID)
	if err != nil {
		return Session{}, err
	}
	user, err := s.repository.CreateUserWithSession(ctx, NewUser{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  name,
		Nickname:     nickname,
	}, newSession)
	if err != nil {
		return Session{}, err
	}
	return s.finishSession(user, refresh, newSession.ExpiresAt)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (Session, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return Session{}, ErrInvalidLogin
	}
	user, lookupErr := s.repository.UserByEmail(ctx, email)
	if lookupErr != nil && !errors.Is(lookupErr, ErrInvalidLogin) {
		return Session{}, lookupErr
	}
	passwordHash := user.PasswordHash
	if errors.Is(lookupErr, ErrInvalidLogin) {
		passwordHash = s.dummyPasswordHash
	}
	valid, err := VerifyPassword(input.Password, passwordHash)
	if err != nil {
		if errors.Is(lookupErr, ErrInvalidLogin) {
			return Session{}, ErrInvalidLogin
		}
		return Session{}, fmt.Errorf("verify stored password: %w", err)
	}
	if lookupErr != nil || !valid {
		return Session{}, ErrInvalidLogin
	}
	refresh, newSession, err := s.newRefreshSession(user.ID)
	if err != nil {
		return Session{}, err
	}
	if err := s.repository.CreateSession(ctx, newSession); err != nil {
		return Session{}, err
	}
	return s.finishSession(user.User, refresh, newSession.ExpiresAt)
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (Session, error) {
	if rawRefreshToken == "" {
		return Session{}, ErrUnauthorized
	}
	nextRaw, _, err := NewRefreshToken()
	if err != nil {
		return Session{}, err
	}
	nextHash := HashRefreshToken(nextRaw)
	next := NewRefreshSession{
		ID:        uuid.New(),
		TokenHash: nextHash,
		ExpiresAt: s.now().UTC().Add(s.refreshTTL),
	}
	user, err := s.repository.RotateSession(ctx, HashRefreshToken(rawRefreshToken), next)
	if err != nil {
		return Session{}, err
	}
	return s.finishSession(user, nextRaw, next.ExpiresAt)
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	return s.repository.RevokeSession(ctx, HashRefreshToken(rawRefreshToken))
}

func (s *Service) User(ctx context.Context, userID uuid.UUID) (User, error) {
	return s.repository.UserByID(ctx, userID)
}

func (s *Service) UpdateProfile(
	ctx context.Context,
	userID uuid.UUID,
	input UpdateProfileInput,
) (User, error) {
	name, err := normalizeDisplayName(input.DisplayName)
	if err != nil {
		return User{}, err
	}
	nickname, err := normalizeNickname(input.Nickname)
	if err != nil {
		return User{}, err
	}
	avatarURL, err := normalizeAvatarURL(input.AvatarURL)
	if err != nil {
		return User{}, err
	}
	return s.repository.UpdateProfile(ctx, userID, name, nickname, avatarURL)
}

func (s *Service) newRefreshSession(userID uuid.UUID) (string, NewRefreshSession, error) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return "", NewRefreshSession{}, err
	}
	return raw, NewRefreshSession{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: s.now().UTC().Add(s.refreshTTL),
	}, nil
}

func (s *Service) finishSession(user User, refresh string, refreshUntil time.Time) (Session, error) {
	access, accessUntil, err := s.tokens.CreateAccessToken(user.ID)
	if err != nil {
		return Session{}, err
	}
	return Session{
		User:             user,
		AccessToken:      access,
		AccessTokenUntil: accessUntil,
		RefreshToken:     refresh,
		RefreshUntil:     refreshUntil,
	}, nil
}

func normalizeEmail(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 {
		return "", fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	return value, nil
}

func normalizeDisplayName(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 80 {
		return "", fmt.Errorf("%w: display name must contain 1–80 characters", ErrInvalidInput)
	}
	return value, nil
}

func normalizeNickname(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) < 3 || len(value) > 24 {
		return "", fmt.Errorf("%w: nickname must contain 3–24 characters", ErrInvalidInput)
	}
	if value[0] < 'a' || value[0] > 'z' {
		return "", fmt.Errorf("%w: nickname must start with a letter", ErrInvalidInput)
	}
	previousUnderscore := false
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' || character == '_'
		if !valid || character == '_' && previousUnderscore {
			return "", fmt.Errorf("%w: nickname may contain letters, digits, and single underscores", ErrInvalidInput)
		}
		previousUnderscore = character == '_'
	}
	if previousUnderscore {
		return "", fmt.Errorf("%w: nickname must end with a letter or digit", ErrInvalidInput)
	}
	return value, nil
}

func normalizeAvatarURL(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	if len(value) > 2048 {
		return nil, fmt.Errorf("%w: avatar URL must contain at most 2048 bytes", ErrInvalidInput)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("%w: avatar URL must be an absolute HTTPS URL without credentials", ErrInvalidInput)
	}
	return &value, nil
}
