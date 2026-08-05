package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/google/uuid"
)

type repositoryStub struct {
	avatar       Photo
	meetingPhoto Photo
	planPhoto    Photo
	version      int64
}

func (*repositoryStub) GetUserAvatar(context.Context, uuid.UUID, uuid.UUID) (Photo, error) {
	return Photo{}, ErrNotFound
}

func (s *repositoryStub) PutUserAvatar(_ context.Context, _ uuid.UUID, photo Photo) (AvatarMutation, error) {
	s.avatar = photo
	revision := int64(1)
	return AvatarMutation{AvatarRevision: &revision, Changed: true}, nil
}

func (*repositoryStub) DeleteUserAvatar(context.Context, uuid.UUID) (AvatarMutation, error) {
	return AvatarMutation{}, nil
}

func (*repositoryStub) GetMeetingPhoto(context.Context, uuid.UUID, uuid.UUID) (Photo, error) {
	return Photo{}, ErrNotFound
}

func (s *repositoryStub) PutMeetingPhoto(
	_ context.Context,
	_, _ uuid.UUID,
	version int64,
	photo Photo,
) (Mutation, error) {
	s.meetingPhoto = photo
	s.version = version
	return Mutation{Version: version + 1, Changed: true}, nil
}

func (*repositoryStub) DeleteMeetingPhoto(
	context.Context, uuid.UUID, uuid.UUID, int64,
) (Mutation, error) {
	return Mutation{}, nil
}

func (*repositoryStub) GetPlanOptionPhoto(
	context.Context, uuid.UUID, uuid.UUID, uuid.UUID,
) (Photo, error) {
	return Photo{}, ErrNotFound
}

func (s *repositoryStub) PutPlanOptionPhoto(
	_ context.Context,
	_, _, _ uuid.UUID,
	version int64,
	photo Photo,
) (Mutation, error) {
	s.planPhoto = photo
	s.version = version
	return Mutation{Version: version + 1, Changed: true}, nil
}

func (*repositoryStub) DeletePlanOptionPhoto(
	context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64,
) (Mutation, error) {
	return Mutation{}, nil
}

func TestPutMeetingPhotoValidatesPNGBytes(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service := NewService(repository)
	content := testPNG(t, 32, 20)
	result, err := service.PutMeetingPhoto(
		context.Background(), uuid.New(), uuid.New(), 4,
		"image/png; charset=binary", content,
	)
	if err != nil {
		t.Fatalf("PutMeetingPhoto() error = %v", err)
	}
	if !result.Changed || result.Version != 5 {
		t.Fatalf("PutMeetingPhoto() = %#v", result)
	}
	if repository.version != 4 ||
		repository.meetingPhoto.ContentType != "image/png" ||
		!bytes.Equal(repository.meetingPhoto.Content, content) ||
		len(repository.meetingPhoto.ContentHash) != 32 {
		t.Fatalf("repository photo = %#v, version = %d", repository.meetingPhoto, repository.version)
	}
}

func TestPutPlanPhotoRejectsMismatchedContentType(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, err := service.PutPlanOptionPhoto(
		context.Background(), uuid.New(), uuid.New(), uuid.New(), 1,
		"image/jpeg", testPNG(t, 1, 1),
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("PutPlanOptionPhoto() error = %v, want ErrInvalidInput", err)
	}
}

func TestPutPhotoRejectsOversizedBodyBeforeDecode(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, err := service.PutMeetingPhoto(
		context.Background(), uuid.New(), uuid.New(), 1,
		"image/png", make([]byte, MaxPhotoBytes+1),
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("PutMeetingPhoto() error = %v, want ErrInvalidInput", err)
	}
}

func TestPutUserAvatarAcceptsBoundedSquareImage(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{}
	service := NewService(repository)
	result, err := service.PutUserAvatar(context.Background(), uuid.New(), "image/png", testPNG(t, 128, 128))
	if err != nil || !result.Changed || result.AvatarRevision == nil {
		t.Fatalf("PutUserAvatar() = %#v, %v", result, err)
	}
	if repository.avatar.ContentType != "image/png" || len(repository.avatar.ContentHash) != 32 {
		t.Fatalf("repository avatar = %#v", repository.avatar)
	}
}

func TestPutUserAvatarRejectsNonSquareImage(t *testing.T) {
	t.Parallel()
	service := NewService(&repositoryStub{})
	_, err := service.PutUserAvatar(context.Background(), uuid.New(), "image/png", testPNG(t, 128, 96))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("PutUserAvatar() error = %v, want ErrInvalidInput", err)
	}
}

func TestDeletePhotoRequiresMeetingVersion(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})
	_, err := service.DeleteMeetingPhoto(context.Background(), uuid.New(), uuid.New(), 0)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("DeleteMeetingPhoto() error = %v, want ErrInvalidInput", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, width, height))
	source.Set(0, 0, color.RGBA{R: 180, G: 42, B: 31, A: 255})
	var result bytes.Buffer
	if err := png.Encode(&result, source); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return result.Bytes()
}
