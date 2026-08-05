package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MaxPhotoBytes      = 3 << 20
	MaxAvatarBytes     = 1 << 20
	MaxAvatarDimension = 1024
	MaxPhotoWidth      = 6000
	MaxPhotoHeight     = 6000
	MaxPhotoPixels     = 24_000_000
)

var (
	ErrInvalidInput    = errors.New("invalid photo input")
	ErrNotFound        = errors.New("photo resource not found")
	ErrNotEditable     = errors.New("photo is no longer editable")
	ErrVersionConflict = errors.New("meeting version conflict")
)

type Photo struct {
	ContentType string
	Content     []byte
	ContentHash []byte
	UpdatedAt   time.Time
}

type Mutation struct {
	Version   int64     `json:"version"`
	Changed   bool      `json:"changed"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AvatarMutation struct {
	AvatarRevision *int64    `json:"avatar_revision"`
	Changed        bool      `json:"changed"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Repository interface {
	GetUserAvatar(context.Context, uuid.UUID, uuid.UUID) (Photo, error)
	PutUserAvatar(context.Context, uuid.UUID, Photo) (AvatarMutation, error)
	DeleteUserAvatar(context.Context, uuid.UUID) (AvatarMutation, error)
	GetMeetingPhoto(context.Context, uuid.UUID, uuid.UUID) (Photo, error)
	PutMeetingPhoto(context.Context, uuid.UUID, uuid.UUID, int64, Photo) (Mutation, error)
	DeleteMeetingPhoto(context.Context, uuid.UUID, uuid.UUID, int64) (Mutation, error)
	GetPlanOptionPhoto(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Photo, error)
	PutPlanOptionPhoto(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, Photo) (Mutation, error)
	DeletePlanOptionPhoto(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) (Mutation, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) GetUserAvatar(ctx context.Context, viewerID, userID uuid.UUID) (Photo, error) {
	return s.repository.GetUserAvatar(ctx, viewerID, userID)
}

func (s *Service) PutUserAvatar(
	ctx context.Context,
	userID uuid.UUID,
	contentType string,
	content []byte,
) (AvatarMutation, error) {
	photo, err := validateAvatar(contentType, content)
	if err != nil {
		return AvatarMutation{}, err
	}
	return s.repository.PutUserAvatar(ctx, userID, photo)
}

func (s *Service) DeleteUserAvatar(ctx context.Context, userID uuid.UUID) (AvatarMutation, error) {
	return s.repository.DeleteUserAvatar(ctx, userID)
}

func (s *Service) GetMeetingPhoto(
	ctx context.Context,
	userID, meetingID uuid.UUID,
) (Photo, error) {
	return s.repository.GetMeetingPhoto(ctx, userID, meetingID)
}

func (s *Service) PutMeetingPhoto(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	expectedVersion int64,
	contentType string,
	content []byte,
) (Mutation, error) {
	photo, err := validatePhoto(expectedVersion, contentType, content)
	if err != nil {
		return Mutation{}, err
	}
	return s.repository.PutMeetingPhoto(ctx, ownerID, meetingID, expectedVersion, photo)
}

func (s *Service) DeleteMeetingPhoto(
	ctx context.Context,
	ownerID, meetingID uuid.UUID,
	expectedVersion int64,
) (Mutation, error) {
	if expectedVersion < 1 {
		return Mutation{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	return s.repository.DeleteMeetingPhoto(ctx, ownerID, meetingID, expectedVersion)
}

func (s *Service) GetPlanOptionPhoto(
	ctx context.Context,
	userID, meetingID, optionID uuid.UUID,
) (Photo, error) {
	return s.repository.GetPlanOptionPhoto(ctx, userID, meetingID, optionID)
}

func (s *Service) PutPlanOptionPhoto(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	expectedVersion int64,
	contentType string,
	content []byte,
) (Mutation, error) {
	photo, err := validatePhoto(expectedVersion, contentType, content)
	if err != nil {
		return Mutation{}, err
	}
	return s.repository.PutPlanOptionPhoto(ctx, ownerID, meetingID, optionID, expectedVersion, photo)
}

func (s *Service) DeletePlanOptionPhoto(
	ctx context.Context,
	ownerID, meetingID, optionID uuid.UUID,
	expectedVersion int64,
) (Mutation, error) {
	if expectedVersion < 1 {
		return Mutation{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	return s.repository.DeletePlanOptionPhoto(ctx, ownerID, meetingID, optionID, expectedVersion)
}

func validatePhoto(expectedVersion int64, claimedContentType string, content []byte) (Photo, error) {
	if expectedVersion < 1 {
		return Photo{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	if len(content) == 0 || len(content) > MaxPhotoBytes {
		return Photo{}, fmt.Errorf("%w: photo must contain at most %d bytes", ErrInvalidInput, MaxPhotoBytes)
	}

	detectedContentType := http.DetectContentType(content)
	if detectedContentType != "image/jpeg" && detectedContentType != "image/png" {
		return Photo{}, fmt.Errorf("%w: photo must be JPEG or PNG", ErrInvalidInput)
	}
	claimedContentType = strings.ToLower(strings.TrimSpace(strings.SplitN(claimedContentType, ";", 2)[0]))
	if claimedContentType != "" && claimedContentType != detectedContentType {
		return Photo{}, fmt.Errorf("%w: photo content type does not match its bytes", ErrInvalidInput)
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return Photo{}, fmt.Errorf("%w: photo cannot be decoded", ErrInvalidInput)
	}
	if config.Width < 1 ||
		config.Height < 1 ||
		config.Width > MaxPhotoWidth ||
		config.Height > MaxPhotoHeight ||
		config.Width > MaxPhotoPixels/config.Height {
		return Photo{}, fmt.Errorf(
			"%w: photo dimensions must not exceed %dx%d or %d pixels",
			ErrInvalidInput, MaxPhotoWidth, MaxPhotoHeight, MaxPhotoPixels,
		)
	}

	hash := sha256.Sum256(content)
	return Photo{
		ContentType: detectedContentType,
		Content:     content,
		ContentHash: hash[:],
	}, nil
}

func validateAvatar(claimedContentType string, content []byte) (Photo, error) {
	if len(content) == 0 || len(content) > MaxAvatarBytes {
		return Photo{}, fmt.Errorf("%w: avatar must contain at most %d bytes", ErrInvalidInput, MaxAvatarBytes)
	}
	detectedContentType := http.DetectContentType(content)
	if detectedContentType != "image/jpeg" && detectedContentType != "image/png" {
		return Photo{}, fmt.Errorf("%w: avatar must be JPEG or PNG", ErrInvalidInput)
	}
	claimedContentType = strings.ToLower(strings.TrimSpace(strings.SplitN(claimedContentType, ";", 2)[0]))
	if claimedContentType != "" && claimedContentType != detectedContentType {
		return Photo{}, fmt.Errorf("%w: avatar content type does not match its bytes", ErrInvalidInput)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return Photo{}, fmt.Errorf("%w: avatar cannot be decoded", ErrInvalidInput)
	}
	if config.Width < 1 || config.Height < 1 || config.Width != config.Height || config.Width > MaxAvatarDimension {
		return Photo{}, fmt.Errorf("%w: avatar must be square and at most %dx%d", ErrInvalidInput, MaxAvatarDimension, MaxAvatarDimension)
	}
	hash := sha256.Sum256(content)
	return Photo{ContentType: detectedContentType, Content: content, ContentHash: hash[:]}, nil
}
