package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	t.Parallel()

	manager := NewTokenManager("this-test-secret-is-longer-than-thirty-two-bytes", time.Minute)
	now := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	userID := uuid.New()

	raw, expiresAt, err := manager.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}
	if want := now.Add(time.Minute); !expiresAt.Equal(want) {
		t.Fatalf("expiresAt = %v, want %v", expiresAt, want)
	}

	// Parsing validates against the real clock used by jwt. Create a second manager
	// with a real-time token for the round trip.
	manager = NewTokenManager("this-test-secret-is-longer-than-thirty-two-bytes", time.Minute)
	raw, realExpiresAt, err := manager.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken(real time) error = %v", err)
	}
	got, err := manager.ParseAccessToken(raw)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}
	if got != userID {
		t.Fatalf("ParseAccessToken() = %v, want %v", got, userID)
	}
	got, parsedExpiresAt, err := manager.ParseAccessTokenWithExpiry(raw)
	if err != nil {
		t.Fatalf("ParseAccessTokenWithExpiry() error = %v", err)
	}
	if got != userID || !parsedExpiresAt.Equal(realExpiresAt) {
		t.Fatalf(
			"ParseAccessTokenWithExpiry() = (%v, %v), want (%v, %v)",
			got, parsedExpiresAt, userID, realExpiresAt,
		)
	}
}

func TestRefreshTokenIsRandomAndHashable(t *testing.T) {
	t.Parallel()

	first, firstHash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}
	second, _, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() second error = %v", err)
	}
	if first == second {
		t.Fatal("two refresh tokens are equal")
	}
	if got := HashRefreshToken(first); string(got) != string(firstHash) {
		t.Fatal("HashRefreshToken() does not match generated hash")
	}
}

func TestAccessTokenRequiresExpiry(t *testing.T) {
	t.Parallel()

	const secret = "this-test-secret-is-longer-than-thirty-two-bytes"
	manager := NewTokenManager(secret, time.Minute)
	userID := uuid.New()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{
		UserID: userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "ryden",
			Subject:  userID.String(),
			Audience: jwt.ClaimStrings{"ryden-web"},
		},
	})
	raw, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if _, err := manager.ParseAccessToken(raw); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ParseAccessToken(without expiry) error = %v, want ErrUnauthorized", err)
	}
}
