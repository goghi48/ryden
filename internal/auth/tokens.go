package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

type accessClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{
		secret: []byte(secret),
		issuer: "ryden",
		ttl:    ttl,
		now:    time.Now,
	}
}

func (m *TokenManager) CreateAccessToken(userID uuid.UUID) (string, time.Time, error) {
	now := m.now().UTC()
	expiresAt := now.Add(m.ttl).Truncate(time.Second)
	claims := accessClaims{
		UserID: userID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"ryden-web"},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *TokenManager) ParseAccessToken(raw string) (uuid.UUID, error) {
	userID, _, err := m.ParseAccessTokenWithExpiry(raw)
	return userID, err
}

func (m *TokenManager) ParseAccessTokenWithExpiry(raw string) (uuid.UUID, time.Time, error) {
	token, err := jwt.ParseWithClaims(
		raw,
		&accessClaims{},
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return m.secret, nil
		},
		jwt.WithAudience("ryden-web"),
		jwt.WithIssuer(m.issuer),
		jwt.WithLeeway(10*time.Second),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return uuid.Nil, time.Time{}, ErrUnauthorized
	}
	claims, ok := token.Claims.(*accessClaims)
	if !ok || claims.ExpiresAt == nil {
		return uuid.Nil, time.Time{}, ErrUnauthorized
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil || claims.Subject != claims.UserID {
		return uuid.Nil, time.Time{}, ErrUnauthorized
	}
	return userID, claims.ExpiresAt.Time, nil
}

func NewRefreshToken() (raw string, hash []byte, err error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, fmt.Errorf("generate refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(value)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

func HashRefreshToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
