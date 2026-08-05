package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ryden-app/ryden/internal/attendance"
	"github.com/ryden-app/ryden/internal/auth"
	"github.com/ryden-app/ryden/internal/availability"
	"github.com/ryden-app/ryden/internal/decision"
	"github.com/ryden-app/ryden/internal/friendship"
	"github.com/ryden-app/ryden/internal/live"
	"github.com/ryden-app/ryden/internal/media"
	"github.com/ryden-app/ryden/internal/meeting"
	"github.com/ryden-app/ryden/internal/meetinginvite"
	"github.com/ryden-app/ryden/internal/migrations"
	"github.com/ryden-app/ryden/internal/note"
	"github.com/ryden-app/ryden/internal/poll"
	"github.com/ryden-app/ryden/internal/preparation"
)

func TestAuthenticatedMeetingFlow(t *testing.T) {
	databaseURL := os.Getenv("RYDEN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("RYDEN_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()
	var databaseName string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatalf("read integration database name: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(databaseName), "_test") {
		t.Fatalf(
			"refusing destructive integration test on database %q: RYDEN_TEST_DATABASE_URL must select a database whose name ends with _test",
			databaseName,
		)
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("migrations.Apply() error = %v", err)
	}
	if _, err := pool.Exec(ctx,
		"TRUNCATE idempotency_keys, meeting_participants, meetings, refresh_sessions, users CASCADE",
	); err != nil {
		t.Fatalf("truncate test database: %v", err)
	}

	tokenManager := auth.NewTokenManager(
		"integration-test-secret-is-longer-than-thirty-two-bytes",
		15*time.Minute,
	)
	authService, err := auth.NewService(auth.NewPostgresRepository(pool), tokenManager, 24*time.Hour)
	if err != nil {
		t.Fatalf("auth.NewService() error = %v", err)
	}
	meetingService := meeting.NewService(meeting.NewPostgresRepository(pool))
	friendshipService := friendship.NewService(friendship.NewPostgresRepository(pool))
	meetingInviteService := meetinginvite.NewService(meetinginvite.NewPostgresRepository(pool))
	pollService := poll.NewService(poll.NewPostgresRepository(pool))
	availabilityService := availability.NewService(availability.NewPostgresRepository(pool))
	attendanceService := attendance.NewService(attendance.NewPostgresRepository(pool))
	noteService := note.NewService(note.NewPostgresRepository(pool))
	decisionService := decision.NewService(decision.NewPostgresRepository(pool))
	mediaService := media.NewService(media.NewPostgresRepository(pool))
	preparationService := preparation.NewService(preparation.NewPostgresRepository(pool))

	owner, err := authService.Register(ctx, auth.RegisterInput{
		Email: "owner@example.test", Password: "correct horse battery staple", DisplayName: "Анна", Nickname: "anna_owner",
	})
	if err != nil {
		t.Fatalf("Register(owner) error = %v", err)
	}
	avatarURL := "https://images.example.test/avatars/anna.png"
	updatedOwner, err := authService.UpdateProfile(ctx, owner.User.ID, auth.UpdateProfileInput{
		DisplayName: "Анна Р.",
		Nickname:    "anna_owner",
		AvatarURL:   &avatarURL,
	})
	if err != nil {
		t.Fatalf("UpdateProfile(owner) error = %v", err)
	}
	if updatedOwner.DisplayName != "Анна Р." ||
		updatedOwner.AvatarURL == nil ||
		*updatedOwner.AvatarURL != avatarURL {
		t.Fatalf("UpdateProfile(owner) = %#v", updatedOwner)
	}
	owner.User = updatedOwner
	rotated, err := authService.Refresh(ctx, owner.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh(owner) error = %v", err)
	}
	if rotated.RefreshToken == owner.RefreshToken {
		t.Fatal("Refresh(owner) returned the prior refresh token")
	}
	if _, err := authService.Refresh(ctx, owner.RefreshToken); !errors.Is(err, auth.ErrSessionNotActive) {
		t.Fatalf("Refresh(reused token) error = %v, want ErrSessionNotActive", err)
	}
	if err := authService.Logout(ctx, rotated.RefreshToken); err != nil {
		t.Fatalf("Logout(rotated token) error = %v", err)
	}
	if _, err := authService.Refresh(ctx, rotated.RefreshToken); !errors.Is(err, auth.ErrSessionNotActive) {
		t.Fatalf("Refresh(logged out token) error = %v, want ErrSessionNotActive", err)
	}
	other, err := authService.Register(ctx, auth.RegisterInput{
		Email: "other@example.test", Password: "another safe password", DisplayName: "Дима", Nickname: "dima_other",
	})
	if err != nil {
		t.Fatalf("Register(other) error = %v", err)
	}
	searchResults, err := friendshipService.Search(ctx, owner.User.ID, "dima", 20)
	if err != nil || len(searchResults) != 1 || searchResults[0].ID != other.User.ID {
		t.Fatalf("Search(friend) = %#v, %v", searchResults, err)
	}
	sent, err := friendshipService.Send(ctx, owner.User.ID, other.User.ID)
	if err != nil || !sent.Changed {
		t.Fatalf("Send(friend request) = %#v, %v", sent, err)
	}
	otherFriends, err := friendshipService.Overview(ctx, other.User.ID, 50, 0)
	if err != nil || len(otherFriends.Incoming.Items) != 1 {
		t.Fatalf("Overview(incoming) = %#v, %v", otherFriends, err)
	}
	accepted, err := friendshipService.Accept(ctx, other.User.ID, otherFriends.Incoming.Items[0].RequestID)
	if err != nil || !accepted.Changed {
		t.Fatalf("Accept(friend request) = %#v, %v", accepted, err)
	}
	ownerFriends, err := friendshipService.Overview(ctx, owner.User.ID, 50, 0)
	if err != nil || len(ownerFriends.Friends.Items) != 1 || ownerFriends.Friends.Items[0].UserID != other.User.ID {
		t.Fatalf("Overview(friends) = %#v, %v", ownerFriends, err)
	}

	locationName := "Дом Анны"
	locationURL := "https://maps.example.test/anna"
	coverURL := "https://images.example.test/meetings/game-night.jpg"
	created, replayed, err := meetingService.Create(
		ctx,
		owner.User.ID,
		"integration-create-1",
		meeting.CreateInput{
			Title: "Вечер настольных игр", Description: "Выберем игру позже",
			EventType: "game_night", CoverURL: &coverURL,
			LocationName: &locationName, LocationURL: &locationURL,
			Timezone: "Asia/Novosibirsk",
		},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if replayed {
		t.Fatal("first Create() replayed = true")
	}
	again, replayed, err := meetingService.Create(
		ctx,
		owner.User.ID,
		"integration-create-1",
		meeting.CreateInput{
			Title: "Вечер настольных игр", Description: "Выберем игру позже",
			EventType: "game_night", CoverURL: &coverURL,
			LocationName: &locationName, LocationURL: &locationURL,
			Timezone: "Asia/Novosibirsk",
		},
	)
	if err != nil {
		t.Fatalf("Create(retry) error = %v", err)
	}
	if !replayed || again.ID != created.ID {
		t.Fatalf("Create(retry) = (%v, %v), want same meeting and replayed", again.ID, replayed)
	}

	page, err := meetingService.List(ctx, owner.User.ID, 20, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ParticipantRole != "owner" {
		t.Fatalf("List() = %#v, want one owned meeting", page.Items)
	}
	if page.Items[0].EventType != "game_night" ||
		page.Items[0].CoverURL == nil ||
		*page.Items[0].CoverURL != coverURL ||
		page.Items[0].LocationName == nil ||
		*page.Items[0].LocationName != locationName ||
		page.Items[0].LocationURL == nil ||
		*page.Items[0].LocationURL != locationURL {
		t.Fatalf("List() meeting metadata = %#v", page.Items[0])
	}

	updatedLocationName := "У Анны"
	updated, err := meetingService.Update(ctx, owner.User.ID, created.ID, meeting.UpdateInput{
		Title: "Игры и пицца", Description: "Выбираем план вместе", EventType: "dinner",
		CoverURL: nil, LocationName: &updatedLocationName, LocationURL: nil,
		ExpectedVersion: created.Version,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Title != "Игры и пицца" ||
		updated.Description != "Выбираем план вместе" ||
		updated.EventType != "dinner" ||
		updated.CoverURL != nil ||
		updated.LocationName == nil ||
		*updated.LocationName != updatedLocationName ||
		updated.LocationURL != nil ||
		updated.Version != created.Version+1 {
		t.Fatalf("Update() = %#v", updated)
	}
	if _, err := meetingService.Update(ctx, owner.User.ID, created.ID, meeting.UpdateInput{
		Title: "Устаревшая правка", EventType: "other", ExpectedVersion: created.Version,
	}); !errors.Is(err, meeting.ErrVersionConflict) {
		t.Fatalf("Update(stale version) error = %v, want ErrVersionConflict", err)
	}
	if _, err := meetingService.Update(ctx, other.User.ID, created.ID, meeting.UpdateInput{
		Title: "Чужая правка", EventType: "other", ExpectedVersion: updated.Version,
	}); !errors.Is(err, meeting.ErrNotFound) {
		t.Fatalf("Update(other user) error = %v, want ErrNotFound", err)
	}

	meetingPhoto, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatalf("decode meeting photo fixture: %v", err)
	}
	photoMutation, err := mediaService.PutMeetingPhoto(
		ctx, owner.User.ID, created.ID, updated.Version, "image/png", meetingPhoto,
	)
	if err != nil || !photoMutation.Changed || photoMutation.Version != updated.Version+1 {
		t.Fatalf("PutMeetingPhoto() = (%#v, %v)", photoMutation, err)
	}
	replayedPhoto, err := mediaService.PutMeetingPhoto(
		ctx, owner.User.ID, created.ID, updated.Version, "image/png", meetingPhoto,
	)
	if err != nil || replayedPhoto.Changed || replayedPhoto.Version != photoMutation.Version {
		t.Fatalf("PutMeetingPhoto(retry) = (%#v, %v)", replayedPhoto, err)
	}
	storedMeetingPhoto, err := mediaService.GetMeetingPhoto(ctx, owner.User.ID, created.ID)
	if err != nil ||
		storedMeetingPhoto.ContentType != "image/png" ||
		!bytes.Equal(storedMeetingPhoto.Content, meetingPhoto) {
		t.Fatalf("GetMeetingPhoto() = (%#v, %v)", storedMeetingPhoto, err)
	}
	if _, err := mediaService.GetMeetingPhoto(ctx, other.User.ID, created.ID); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("GetMeetingPhoto(other user) error = %v, want ErrNotFound", err)
	}

	_, err = meetingService.Get(ctx, other.User.ID, created.ID)
	if !errors.Is(err, meeting.ErrNotFound) {
		t.Fatalf("Get(other user) error = %v, want ErrNotFound", err)
	}

	planOptionIDs := make([]uuid.UUID, 0, 2)
	for index, title := range []string{"Настольные игры", "Кино и ужин"} {
		option, optionReplayed, err := meetingService.AddPlanOption(
			ctx, owner.User.ID, created.ID, "plan-option-key-"+title,
			meeting.AddPlanOptionInput{Title: title},
		)
		if err != nil {
			t.Fatalf("AddPlanOption(%d) error = %v", index, err)
		}
		if optionReplayed || option.Position != int16(index) {
			t.Fatalf("AddPlanOption(%d) = (%#v, %v)", index, option, optionReplayed)
		}
		planOptionIDs = append(planOptionIDs, option.ID)
	}
	detailWithPlans, err := meetingService.Get(ctx, owner.User.ID, created.ID)
	if err != nil {
		t.Fatalf("Get(before plan photo) error = %v", err)
	}
	planPhotoMutation, err := mediaService.PutPlanOptionPhoto(
		ctx, owner.User.ID, created.ID, planOptionIDs[0],
		detailWithPlans.Version, "image/png", meetingPhoto,
	)
	if err != nil || !planPhotoMutation.Changed {
		t.Fatalf("PutPlanOptionPhoto() = (%#v, %v)", planPhotoMutation, err)
	}
	detailWithPhotos, err := meetingService.Get(ctx, owner.User.ID, created.ID)
	if err != nil ||
		!detailWithPhotos.HasPhoto ||
		len(detailWithPhotos.PlanOptions) != 2 ||
		!detailWithPhotos.PlanOptions[0].HasPhoto {
		t.Fatalf("Get(photo flags) = (%#v, %v)", detailWithPhotos, err)
	}

	firstSecret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 32))
	if _, _, err := meetingService.CreateInvitation(
		ctx, owner.User.ID, created.ID, "invite-key-before-times",
		meeting.CreateInvitationInput{Secret: firstSecret},
	); !errors.Is(err, meeting.ErrSetupIncomplete) {
		t.Fatalf("CreateInvitation(incomplete) error = %v, want ErrSetupIncomplete", err)
	}

	start := time.Now().UTC().Add(48 * time.Hour).Truncate(5 * time.Minute).Add(5 * time.Minute)
	timeOptionIDs := make([]uuid.UUID, 0, 2)
	for index := range 2 {
		var planOptionID *uuid.UUID
		if index == 1 {
			planOptionID = &planOptionIDs[1]
		}
		option, optionReplayed, err := meetingService.AddTimeOption(
			ctx, owner.User.ID, created.ID, "time-option-key-"+string(rune('a'+index)),
			meeting.AddTimeOptionInput{
				PlanOptionID: planOptionID,
				StartsAt:     start.Add(time.Duration(index) * 24 * time.Hour),
				EndsAt:       timePointer(start.Add(time.Duration(index)*24*time.Hour + 3*time.Hour)),
			},
		)
		if err != nil {
			t.Fatalf("AddTimeOption(%d) error = %v", index, err)
		}
		if optionReplayed || option.Position != int16(index) {
			t.Fatalf("AddTimeOption(%d) = (%#v, %v)", index, option, optionReplayed)
		}
		timeOptionIDs = append(timeOptionIDs, option.ID)
	}
	scopedDetail, err := meetingService.Get(ctx, owner.User.ID, created.ID)
	if err != nil {
		t.Fatalf("Get(scoped times) error = %v", err)
	}
	if scopedDetail.TimeOptions[0].PlanOptionID != nil ||
		scopedDetail.TimeOptions[1].PlanOptionID == nil ||
		*scopedDetail.TimeOptions[1].PlanOptionID != planOptionIDs[1] {
		t.Fatalf("time option scopes = %#v", scopedDetail.TimeOptions)
	}
	updatedPlan, err := meetingService.UpdatePlanOption(
		ctx, owner.User.ID, created.ID, planOptionIDs[0],
		meeting.UpdatePlanOptionInput{
			Title: "Настольные игры и пицца", Description: "Исправленный вариант",
			ExpectedVersion: scopedDetail.Version,
		},
	)
	if err != nil {
		t.Fatalf("UpdatePlanOption() error = %v", err)
	}
	if updatedPlan.Title != "Настольные игры и пицца" ||
		updatedPlan.Description != "Исправленный вариант" {
		t.Fatalf("UpdatePlanOption() = %#v", updatedPlan)
	}
	if _, err := meetingService.UpdateTimeOption(
		ctx, owner.User.ID, created.ID, timeOptionIDs[1],
		meeting.UpdateTimeOptionInput{
			PlanOptionID:    &planOptionIDs[1],
			StartsAt:        start.Add(24*time.Hour + 30*time.Minute),
			EndsAt:          timePointer(start.Add(27*time.Hour + 30*time.Minute)),
			ExpectedVersion: scopedDetail.Version,
		},
	); !errors.Is(err, meeting.ErrVersionConflict) {
		t.Fatalf("UpdateTimeOption(stale) error = %v, want ErrVersionConflict", err)
	}
	updatedTime, err := meetingService.UpdateTimeOption(
		ctx, owner.User.ID, created.ID, timeOptionIDs[1],
		meeting.UpdateTimeOptionInput{
			PlanOptionID:    &planOptionIDs[1],
			StartsAt:        start.Add(24*time.Hour + 30*time.Minute),
			EndsAt:          timePointer(start.Add(27*time.Hour + 30*time.Minute)),
			ExpectedVersion: scopedDetail.Version + 1,
		},
	)
	if err != nil {
		t.Fatalf("UpdateTimeOption() error = %v", err)
	}
	if updatedTime.PlanOptionID == nil ||
		*updatedTime.PlanOptionID != planOptionIDs[1] ||
		!updatedTime.StartsAt.Equal(start.Add(24*time.Hour+30*time.Minute)) {
		t.Fatalf("UpdateTimeOption() = %#v", updatedTime)
	}
	temporaryPlan, _, err := meetingService.AddPlanOption(
		ctx, owner.User.ID, created.ID, "temporary-plan-key",
		meeting.AddPlanOptionInput{Title: "Временный план"},
	)
	if err != nil {
		t.Fatalf("AddPlanOption(temporary) error = %v", err)
	}
	if _, _, err := meetingService.AddTimeOption(
		ctx, owner.User.ID, created.ID, "temporary-time-key",
		meeting.AddTimeOptionInput{
			PlanOptionID: &temporaryPlan.ID,
			StartsAt:     start.Add(72 * time.Hour),
			EndsAt:       timePointer(start.Add(75 * time.Hour)),
		},
	); err != nil {
		t.Fatalf("AddTimeOption(temporary) error = %v", err)
	}
	if err := meetingService.DeletePlanOption(
		ctx, owner.User.ID, created.ID, temporaryPlan.ID,
	); err != nil {
		t.Fatalf("DeletePlanOption(temporary) error = %v", err)
	}
	afterCascade, err := meetingService.Get(ctx, owner.User.ID, created.ID)
	if err != nil {
		t.Fatalf("Get(after plan cascade) error = %v", err)
	}
	if len(afterCascade.PlanOptions) != 2 || len(afterCascade.TimeOptions) != 2 {
		t.Fatalf("cascade counts = plans %d, times %d", len(afterCascade.PlanOptions), len(afterCascade.TimeOptions))
	}

	createdPoll, pollReplayed, err := pollService.Create(
		ctx, owner.User.ID, created.ID, "integration-poll-1", poll.CreateInput{
			Question: "Что взять с собой?", ResponseMode: "multiple",
			AllowRevote: true, Options: []string{"Воду", "Плед", "Игры"},
		},
	)
	if err != nil {
		t.Fatalf("CreatePoll() error = %v", err)
	}
	if pollReplayed || len(createdPoll.Options) != 3 {
		t.Fatalf("CreatePoll() = (%#v, %v)", createdPoll, pollReplayed)
	}
	replayedPoll, pollReplayed, err := pollService.Create(
		ctx, owner.User.ID, created.ID, "integration-poll-1", poll.CreateInput{
			Question: "Что взять с собой?", ResponseMode: "multiple",
			AllowRevote: true, Options: []string{"Воду", "Плед", "Игры"},
		},
	)
	if err != nil || !pollReplayed || replayedPoll.ID != createdPoll.ID {
		t.Fatalf("CreatePoll(retry) = (%#v, %v, %v)", replayedPoll, pollReplayed, err)
	}

	invitation, invitationReplayed, err := meetingService.CreateInvitation(
		ctx, owner.User.ID, created.ID, "invite-key-first", meeting.CreateInvitationInput{Secret: firstSecret},
	)
	if err != nil {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
	if invitationReplayed || !invitation.ExpiresAt.After(time.Now()) {
		t.Fatalf("CreateInvitation() = (%#v, %v)", invitation, invitationReplayed)
	}
	_, replayedInvitation, err := meetingService.CreateInvitation(
		ctx, owner.User.ID, created.ID, "invite-key-first", meeting.CreateInvitationInput{Secret: firstSecret},
	)
	if err != nil || !replayedInvitation {
		t.Fatalf("CreateInvitation(retry) = (%v, %v), want replay", replayedInvitation, err)
	}
	if _, err := availabilityService.Respond(
		ctx, other.User.ID, timeOptionIDs[0],
		availability.RespondInput{Status: availability.StatusAvailable},
	); !errors.Is(err, availability.ErrNotFound) {
		t.Fatalf("RespondAvailability(non-participant) error = %v, want ErrNotFound", err)
	}
	if _, err := decisionService.Vote(
		ctx, other.User.ID, created.ID,
		decision.VoteInput{PlanOptionID: &planOptionIDs[0]},
	); !errors.Is(err, decision.ErrNotFound) {
		t.Fatalf("VotePlan(non-participant) error = %v, want ErrNotFound", err)
	}

	detail, joined, err := meetingService.JoinInvitation(
		ctx, other.User.ID, meeting.JoinInvitationInput{Token: firstSecret},
	)
	if err != nil {
		t.Fatalf("JoinInvitation() error = %v", err)
	}
	if !joined || detail.ParticipantRole != "participant" || len(detail.Participants) != 2 {
		t.Fatalf("JoinInvitation() = (%#v, %v)", detail, joined)
	}
	collectingPoll, replayedCollectingPoll, err := pollService.Create(
		ctx, owner.User.ID, created.ID, "integration-poll-live",
		poll.CreateInput{
			Question:     "Нужен запасной план?",
			ResponseMode: "single",
			AllowRevote:  true,
			Options:      []string{"Да", "Нет"},
		},
	)
	if err != nil || replayedCollectingPoll {
		t.Fatalf(
			"CreatePoll(collecting) = (%#v, %v, %v)",
			collectingPoll, replayedCollectingPoll, err,
		)
	}
	liveManager := live.NewManager(
		live.NewPostgresVersionSource(pool),
		nil,
		nil,
		live.Options{PollInterval: 10 * time.Millisecond, PollTimeout: time.Second},
	)
	liveSubscription, err := liveManager.Subscribe(created.ID, detail.Version)
	if err != nil {
		t.Fatalf("Subscribe(live) error = %v", err)
	}
	defer liveSubscription.Close()
	liveCtx, stopLive := context.WithCancel(ctx)
	liveDone := make(chan error, 1)
	go func() { liveDone <- liveManager.Run(liveCtx) }()
	defer func() {
		stopLive()
		if err := <-liveDone; err != nil {
			t.Errorf("live manager stopped with error: %v", err)
		}
	}()

	for _, response := range []struct {
		userID       uuid.UUID
		timeOptionID uuid.UUID
		status       availability.Status
	}{
		{owner.User.ID, timeOptionIDs[0], availability.StatusAvailable},
		{other.User.ID, timeOptionIDs[0], availability.StatusPreferred},
		{owner.User.ID, timeOptionIDs[1], availability.StatusPreferred},
		{other.User.ID, timeOptionIDs[1], availability.StatusPreferred},
	} {
		changed, err := availabilityService.Respond(
			ctx, response.userID, response.timeOptionID,
			availability.RespondInput{Status: response.status},
		)
		if err != nil || !changed {
			t.Fatalf("RespondAvailability(%s) = (%v, %v), want changed", response.status, changed, err)
		}
	}
	select {
	case event := <-liveSubscription.Events:
		if event.Version <= detail.Version {
			t.Fatalf("live update version = %d, want greater than %d", event.Version, detail.Version)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PostgreSQL-backed live update")
	}
	availabilityView, err := availabilityService.List(ctx, owner.User.ID, created.ID)
	if err != nil {
		t.Fatalf("ListAvailability() error = %v", err)
	}
	if len(availabilityView.Recommendations) != 2 ||
		availabilityView.Recommendations[0].TimeOptionID != timeOptionIDs[0] ||
		availabilityView.Recommendations[1].TimeOptionID != timeOptionIDs[1] {
		t.Fatalf("availability recommendations = %#v", availabilityView.Recommendations)
	}
	if availabilityView.Recommendations[0].Provisional ||
		availabilityView.Recommendations[1].Provisional {
		t.Fatal("availability recommendations are provisional after all participants answered")
	}
	for index, vote := range []struct {
		userID       uuid.UUID
		planOptionID *uuid.UUID
		wantChanged  bool
	}{
		{owner.User.ID, &planOptionIDs[0], true},
		{owner.User.ID, &planOptionIDs[0], false},
		{owner.User.ID, &planOptionIDs[1], true},
		{other.User.ID, &planOptionIDs[1], true},
		{other.User.ID, nil, true},
		{other.User.ID, &planOptionIDs[0], true},
	} {
		changed, err := decisionService.Vote(
			ctx, vote.userID, created.ID,
			decision.VoteInput{PlanOptionID: vote.planOptionID},
		)
		if err != nil || changed != vote.wantChanged {
			t.Fatalf("VotePlan(%d) = (%v, %v), want changed %v", index, changed, err, vote.wantChanged)
		}
	}
	planVotes, err := decisionService.List(ctx, owner.User.ID, created.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListPlanVotes() error = %v", err)
	}
	if planVotes.HistoryTotal != 5 || len(planVotes.History) != 5 ||
		planVotes.AnsweredCount != 2 || planVotes.Options[0].VoteCount != 1 ||
		planVotes.Options[1].VoteCount != 1 {
		t.Fatalf("plan vote page = %#v", planVotes)
	}
	changed, err := pollService.Vote(
		ctx, owner.User.ID, createdPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{createdPoll.Options[0].ID}},
	)
	if err != nil || !changed {
		t.Fatalf("VotePoll(owner) = (%v, %v), want changed", changed, err)
	}
	changed, err = pollService.Vote(
		ctx, other.User.ID, createdPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{createdPoll.Options[0].ID, createdPoll.Options[1].ID}},
	)
	if err != nil || !changed {
		t.Fatalf("VotePoll(participant) = (%v, %v), want changed", changed, err)
	}
	changed, err = pollService.Vote(
		ctx, other.User.ID, createdPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{createdPoll.Options[1].ID, createdPoll.Options[0].ID}},
	)
	if err != nil || changed {
		t.Fatalf("VotePoll(participant no-op) = (%v, %v), want no change", changed, err)
	}
	polls, err := pollService.List(ctx, other.User.ID, created.ID)
	if err != nil {
		t.Fatalf("ListPolls() error = %v", err)
	}
	if len(polls) != 2 || polls[0].Options[0].VoteCount != 2 ||
		!polls[0].Options[1].SelectedByUser ||
		polls[0].ParticipantCount != 2 ||
		polls[0].RespondentCount != 2 ||
		polls[0].TotalSelections != 3 ||
		len(polls[0].Options[0].Voters) != 2 ||
		len(polls[0].Options[1].Voters) != 1 ||
		!polls[1].AcceptingAnswers {
		t.Fatalf("ListPolls() = %#v", polls)
	}
	changed, err = pollService.Vote(
		ctx, owner.User.ID, createdPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{createdPoll.Options[2].ID}},
	)
	if err != nil || !changed {
		t.Fatalf("VotePoll(owner change) = (%v, %v), want changed", changed, err)
	}
	changed, err = pollService.Vote(
		ctx, owner.User.ID, createdPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{}},
	)
	if err != nil || !changed {
		t.Fatalf("VotePoll(owner retract) = (%v, %v), want changed", changed, err)
	}
	pollHistory, err := pollService.History(ctx, other.User.ID, createdPoll.ID, 50, 0)
	if err != nil {
		t.Fatalf("PollHistory() error = %v", err)
	}
	if pollHistory.Total != 4 || len(pollHistory.Items) != 4 ||
		pollHistory.Items[0].Action != "retract" ||
		pollHistory.Items[1].Action != "change" ||
		len(pollHistory.Items[1].PreviousOptionLabels) != 1 ||
		pollHistory.Items[1].PreviousOptionLabels[0] != createdPoll.Options[0].Label ||
		len(pollHistory.Items[1].NewOptionLabels) != 1 ||
		pollHistory.Items[1].NewOptionLabels[0] != createdPoll.Options[2].Label {
		t.Fatalf("poll vote history = %#v", pollHistory)
	}
	selectedPollOptionID := createdPoll.Options[0].ID
	changed, err = pollService.Close(
		ctx, owner.User.ID, created.ID, createdPoll.ID,
		poll.CloseInput{SelectedOptionID: &selectedPollOptionID},
	)
	if err != nil || !changed {
		t.Fatalf("ClosePoll() = (%v, %v), want changed", changed, err)
	}
	changed, err = pollService.Close(
		ctx, owner.User.ID, created.ID, collectingPoll.ID,
		poll.CloseInput{SelectedOptionID: nil},
	)
	if err != nil || !changed {
		t.Fatalf("ClosePoll(without decision) = (%v, %v), want changed", changed, err)
	}
	changed, err = pollService.Close(
		ctx, owner.User.ID, created.ID, collectingPoll.ID,
		poll.CloseInput{SelectedOptionID: nil},
	)
	if err != nil || changed {
		t.Fatalf("ClosePoll(without decision replay) = (%v, %v), want no change", changed, err)
	}
	closedPolls, err := pollService.List(ctx, owner.User.ID, created.ID)
	if err != nil {
		t.Fatalf("ListPolls(after close) error = %v", err)
	}
	if len(closedPolls) != 2 ||
		closedPolls[1].State != "closed" ||
		closedPolls[1].SelectedOptionID != nil ||
		closedPolls[1].AcceptingAnswers {
		t.Fatalf("poll closed without decision = %#v", closedPolls)
	}
	if _, err := pollService.Vote(
		ctx, other.User.ID, createdPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{createdPoll.Options[2].ID}},
	); !errors.Is(err, poll.ErrClosed) {
		t.Fatalf("VotePoll(closed) error = %v, want ErrClosed", err)
	}
	anonymousPoll, replayedAnonymousPoll, err := pollService.Create(
		ctx, other.User.ID, created.ID, "participant-anonymous-poll",
		poll.CreateInput{
			Question: "Кто за ранний старт?", ResponseMode: "single",
			IsAnonymous: true, AllowRevote: false, Options: []string{"За", "Против"},
		},
	)
	if err != nil || replayedAnonymousPoll || !anonymousPoll.IsAnonymous || anonymousPoll.AllowRevote || !anonymousPoll.CanManage {
		t.Fatalf("CreatePoll(participant anonymous) = (%#v, %v, %v)", anonymousPoll, replayedAnonymousPoll, err)
	}
	if changed, err = pollService.Vote(
		ctx, other.User.ID, anonymousPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{anonymousPoll.Options[0].ID}},
	); err != nil || !changed {
		t.Fatalf("VotePoll(anonymous first answer) = (%v, %v), want changed", changed, err)
	}
	if _, err = pollService.Vote(
		ctx, other.User.ID, anonymousPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{anonymousPoll.Options[1].ID}},
	); !errors.Is(err, poll.ErrRevoteDisabled) {
		t.Fatalf("VotePoll(anonymous revote) error = %v, want ErrRevoteDisabled", err)
	}
	anonymousView, err := pollService.List(ctx, owner.User.ID, created.ID)
	if err != nil {
		t.Fatalf("ListPolls(anonymous) error = %v", err)
	}
	listedAnonymous := anonymousView[len(anonymousView)-1]
	if !listedAnonymous.IsAnonymous || len(listedAnonymous.Options[0].Voters) != 0 || listedAnonymous.RespondentCount != 1 {
		t.Fatalf("anonymous poll leaked voters or counts = %#v", listedAnonymous)
	}
	if _, err := pollService.History(ctx, owner.User.ID, anonymousPoll.ID, 50, 0); !errors.Is(err, poll.ErrNotFound) {
		t.Fatalf("History(anonymous) error = %v, want ErrNotFound", err)
	}
	if changed, err = pollService.Close(
		ctx, other.User.ID, created.ID, anonymousPoll.ID, poll.CloseInput{},
	); err != nil || !changed {
		t.Fatalf("ClosePoll(by creator) = (%v, %v), want changed", changed, err)
	}
	_, joinedAgain, err := meetingService.JoinInvitation(
		ctx, other.User.ID, meeting.JoinInvitationInput{Token: firstSecret},
	)
	if err != nil || joinedAgain {
		t.Fatalf("JoinInvitation(retry) = (%v, %v), want idempotent join", joinedAgain, err)
	}
	if _, _, err := meetingService.AddPlanOption(
		ctx, owner.User.ID, created.ID, "plan-after-open",
		meeting.AddPlanOptionInput{Title: "Слишком поздно"},
	); !errors.Is(err, meeting.ErrNotEditable) {
		t.Fatalf("AddPlanOption(after collection) error = %v, want ErrNotEditable", err)
	}
	if _, err := meetingService.Update(ctx, owner.User.ID, created.ID, meeting.UpdateInput{
		Title: "Слишком поздняя правка", EventType: "other", ExpectedVersion: detail.Version,
	}); !errors.Is(err, meeting.ErrNotEditable) {
		t.Fatalf("Update(after collection) error = %v, want ErrNotEditable", err)
	}
	if _, err := meetingService.UpdatePlanOption(
		ctx, owner.User.ID, created.ID, planOptionIDs[0],
		meeting.UpdatePlanOptionInput{
			Title: "Слишком поздний вариант", ExpectedVersion: detail.Version,
		},
	); !errors.Is(err, meeting.ErrNotEditable) {
		t.Fatalf("UpdatePlanOption(after collection) error = %v, want ErrNotEditable", err)
	}

	secondSecret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x72}, 32))
	if _, _, err := meetingService.CreateInvitation(
		ctx, owner.User.ID, created.ID, "invite-key-second",
		meeting.CreateInvitationInput{Secret: secondSecret},
	); err != nil {
		t.Fatalf("CreateInvitation(rotation) error = %v", err)
	}
	if _, _, err := meetingService.JoinInvitation(
		ctx, other.User.ID, meeting.JoinInvitationInput{Token: firstSecret},
	); !errors.Is(err, meeting.ErrInvitationInvalid) {
		t.Fatalf("JoinInvitation(revoked) error = %v, want ErrInvitationInvalid", err)
	}
	rawSecret, _ := base64.RawURLEncoding.DecodeString(secondSecret)
	var rawSecretRows int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM invitations WHERE secret_hash = $1", rawSecret,
	).Scan(&rawSecretRows); err != nil {
		t.Fatalf("query raw invitation secret: %v", err)
	}
	if rawSecretRows != 0 {
		t.Fatal("raw invitation secret was persisted")
	}
	if _, _, err := decisionService.Finalize(
		ctx, owner.User.ID, created.ID,
		decision.FinalizeInput{PlanOptionID: planOptionIDs[0], TimeOptionID: timeOptionIDs[1]},
	); !errors.Is(err, decision.ErrIncompatible) {
		t.Fatalf("Finalize(incompatible) error = %v, want ErrIncompatible", err)
	}
	if _, _, err := decisionService.Finalize(
		ctx, other.User.ID, created.ID,
		decision.FinalizeInput{PlanOptionID: planOptionIDs[1], TimeOptionID: timeOptionIDs[1]},
	); !errors.Is(err, decision.ErrNotFound) {
		t.Fatalf("Finalize(participant) error = %v, want ErrNotFound", err)
	}
	finalDecision, replayedDecision, err := decisionService.Finalize(
		ctx, owner.User.ID, created.ID,
		decision.FinalizeInput{PlanOptionID: planOptionIDs[1], TimeOptionID: timeOptionIDs[1]},
	)
	if err != nil || replayedDecision || finalDecision.State != "scheduled" {
		t.Fatalf("Finalize() = (%#v, %v, %v)", finalDecision, replayedDecision, err)
	}
	_, replayedDecision, err = decisionService.Finalize(
		ctx, owner.User.ID, created.ID,
		decision.FinalizeInput{PlanOptionID: planOptionIDs[1], TimeOptionID: timeOptionIDs[1]},
	)
	if err != nil || !replayedDecision {
		t.Fatalf("Finalize(retry) = (%v, %v), want replay", replayedDecision, err)
	}
	if _, _, err := decisionService.Finalize(
		ctx, owner.User.ID, created.ID,
		decision.FinalizeInput{PlanOptionID: planOptionIDs[0], TimeOptionID: timeOptionIDs[0]},
	); !errors.Is(err, decision.ErrConflict) {
		t.Fatalf("Finalize(other decision) error = %v, want ErrConflict", err)
	}
	scheduled, err := meetingService.Get(ctx, other.User.ID, created.ID)
	if err != nil {
		t.Fatalf("Get(scheduled) error = %v", err)
	}
	if scheduled.State != "scheduled" ||
		scheduled.SelectedPlanOptionID == nil ||
		*scheduled.SelectedPlanOptionID != planOptionIDs[1] ||
		scheduled.SelectedTimeOptionID == nil ||
		*scheduled.SelectedTimeOptionID != timeOptionIDs[1] {
		t.Fatalf("scheduled meeting = %#v", scheduled.Meeting)
	}
	if _, err := availabilityService.Respond(
		ctx, other.User.ID, timeOptionIDs[0],
		availability.RespondInput{Status: availability.StatusIfNeeded},
	); !errors.Is(err, availability.ErrNotEditable) {
		t.Fatalf("RespondAvailability(after decision) error = %v, want ErrNotEditable", err)
	}

	if _, _, err := preparationService.Create(
		ctx, other.User.ID, created.ID, "participant-requirement",
		preparation.CreateInput{Name: "Water", RequiredQuantity: 10},
	); !errors.Is(err, preparation.ErrNotFound) {
		t.Fatalf("CreateRequirement(participant) error = %v, want ErrNotFound", err)
	}
	requirement, replayedRequirement, err := preparationService.Create(
		ctx, owner.User.ID, created.ID, "requirement-water",
		preparation.CreateInput{Name: "Water", RequiredQuantity: 10},
	)
	if err != nil || replayedRequirement {
		t.Fatalf("CreateRequirement() = (%#v, %v, %v)", requirement, replayedRequirement, err)
	}
	requirementReplayResult, replayedRequirement, err := preparationService.Create(
		ctx, owner.User.ID, created.ID, "requirement-water",
		preparation.CreateInput{Name: "Water", RequiredQuantity: 10},
	)
	if err != nil || !replayedRequirement || requirementReplayResult.ID != requirement.ID {
		t.Fatalf(
			"CreateRequirement(retry) = (%#v, %v, %v)",
			requirementReplayResult,
			replayedRequirement,
			err,
		)
	}
	if _, _, err := preparationService.Create(
		ctx, owner.User.ID, created.ID, "requirement-water-duplicate",
		preparation.CreateInput{Name: "water", RequiredQuantity: 5},
	); !errors.Is(err, preparation.ErrDuplicate) {
		t.Fatalf("CreateRequirement(duplicate) error = %v, want ErrDuplicate", err)
	}
	disposableRequirement, _, err := preparationService.Create(
		ctx, owner.User.ID, created.ID, "requirement-disposable",
		preparation.CreateInput{Name: "Paper cups", RequiredQuantity: 6},
	)
	if err != nil {
		t.Fatalf("CreateRequirement(disposable) error = %v", err)
	}
	if _, err := preparationService.Update(
		ctx, other.User.ID, created.ID, disposableRequirement.ID,
		preparation.UpdateInput{Name: "Reusable cups", RequiredQuantity: 4},
	); !errors.Is(err, preparation.ErrNotFound) {
		t.Fatalf("UpdateRequirement(participant) error = %v, want ErrNotFound", err)
	}
	changed, err = preparationService.Update(
		ctx, owner.User.ID, created.ID, disposableRequirement.ID,
		preparation.UpdateInput{Name: "Reusable cups", RequiredQuantity: 4},
	)
	if err != nil || !changed {
		t.Fatalf("UpdateRequirement() = (%v, %v)", changed, err)
	}
	if _, err := preparationService.Delete(
		ctx, other.User.ID, created.ID, disposableRequirement.ID,
	); !errors.Is(err, preparation.ErrNotFound) {
		t.Fatalf("DeleteRequirement(participant) error = %v, want ErrNotFound", err)
	}
	changed, err = preparationService.Delete(
		ctx, owner.User.ID, created.ID, disposableRequirement.ID,
	)
	if err != nil || !changed {
		t.Fatalf("DeleteRequirement() = (%v, %v)", changed, err)
	}
	changed, err = preparationService.SetClaim(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 6},
	)
	if err != nil || !changed {
		t.Fatalf("SetClaim(owner) = (%v, %v)", changed, err)
	}
	if _, err := preparationService.Update(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.UpdateInput{Name: "Water", RequiredQuantity: 5},
	); !errors.Is(err, preparation.ErrQuantityExceeded) {
		t.Fatalf("UpdateRequirement(below claims) error = %v, want ErrQuantityExceeded", err)
	}
	if _, err := preparationService.Delete(
		ctx, owner.User.ID, created.ID, requirement.ID,
	); !errors.Is(err, preparation.ErrHasClaims) {
		t.Fatalf("DeleteRequirement(claimed) error = %v, want ErrHasClaims", err)
	}
	if _, err := preparationService.SetClaim(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 5},
	); !errors.Is(err, preparation.ErrQuantityExceeded) {
		t.Fatalf("SetClaim(over capacity) error = %v, want ErrQuantityExceeded", err)
	}
	changed, err = preparationService.SetClaim(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 4},
	)
	if err != nil || !changed {
		t.Fatalf("SetClaim(participant) = (%v, %v)", changed, err)
	}
	changed, err = preparationService.SetClaim(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 4},
	)
	if err != nil || changed {
		t.Fatalf("SetClaim(retry) = (%v, %v), want unchanged", changed, err)
	}
	if _, err := preparationService.SetStatus(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.StatusInput{Status: preparation.StatusCompleted},
	); !errors.Is(err, preparation.ErrNotFound) {
		t.Fatalf("SetRequirementStatus(participant) error = %v, want ErrNotFound", err)
	}
	changed, err = preparationService.SetStatus(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.StatusInput{Status: preparation.StatusCompleted},
	)
	if err != nil || !changed {
		t.Fatalf("SetRequirementStatus(completed) = (%v, %v)", changed, err)
	}
	if _, err := preparationService.Update(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.UpdateInput{Name: "Drinking water", RequiredQuantity: 10},
	); !errors.Is(err, preparation.ErrNotEditable) {
		t.Fatalf("UpdateRequirement(completed) error = %v, want ErrNotEditable", err)
	}
	if _, err := preparationService.SetClaim(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 0},
	); !errors.Is(err, preparation.ErrNotEditable) {
		t.Fatalf("SetClaim(completed) error = %v, want ErrNotEditable", err)
	}
	changed, err = preparationService.SetStatus(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.StatusInput{Status: preparation.StatusOpen},
	)
	if err != nil || !changed {
		t.Fatalf("SetRequirementStatus(reopen) = (%v, %v)", changed, err)
	}
	if _, err := preparationService.SetClaim(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 0},
	); err != nil {
		t.Fatalf("SetClaim(retract) error = %v", err)
	}
	if _, err := preparationService.SetStatus(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.StatusInput{Status: preparation.StatusCompleted},
	); !errors.Is(err, preparation.ErrNotFullyClaimed) {
		t.Fatalf("SetRequirementStatus(partial) error = %v, want ErrNotFullyClaimed", err)
	}
	if _, err := preparationService.SetClaim(
		ctx, other.User.ID, created.ID, requirement.ID,
		preparation.ClaimInput{Quantity: 4},
	); err != nil {
		t.Fatalf("SetClaim(after reopen) error = %v", err)
	}
	if _, err := preparationService.SetStatus(
		ctx, owner.User.ID, created.ID, requirement.ID,
		preparation.StatusInput{Status: preparation.StatusCompleted},
	); err != nil {
		t.Fatalf("SetRequirementStatus(final) error = %v", err)
	}
	concurrentRequirement, _, err := preparationService.Create(
		ctx, owner.User.ID, created.ID, "requirement-blankets",
		preparation.CreateInput{Name: "Blankets", RequiredQuantity: 10},
	)
	if err != nil {
		t.Fatalf("CreateRequirement(concurrent) error = %v", err)
	}
	startClaims := make(chan struct{})
	claimResults := make(chan error, 2)
	for _, userID := range []uuid.UUID{owner.User.ID, other.User.ID} {
		go func() {
			<-startClaims
			_, claimErr := preparationService.SetClaim(
				ctx, userID, created.ID, concurrentRequirement.ID,
				preparation.ClaimInput{Quantity: 6},
			)
			claimResults <- claimErr
		}()
	}
	close(startClaims)
	successfulClaims := 0
	rejectedClaims := 0
	for range 2 {
		claimErr := <-claimResults
		switch {
		case claimErr == nil:
			successfulClaims++
		case errors.Is(claimErr, preparation.ErrQuantityExceeded):
			rejectedClaims++
		default:
			t.Fatalf("concurrent SetClaim() error = %v", claimErr)
		}
	}
	if successfulClaims != 1 || rejectedClaims != 1 {
		t.Fatalf(
			"concurrent SetClaim() = %d successful, %d rejected",
			successfulClaims,
			rejectedClaims,
		)
	}
	unclaimedRequirement, _, err := preparationService.Create(
		ctx, owner.User.ID, created.ID, "requirement-napkins",
		preparation.CreateInput{Name: "Napkins", RequiredQuantity: 20},
	)
	if err != nil {
		t.Fatalf("CreateRequirement(unclaimed) error = %v", err)
	}
	preparationPage, err := preparationService.List(ctx, other.User.ID, created.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListPreparation() error = %v", err)
	}
	requirementsByID := make(map[uuid.UUID]preparation.Requirement, len(preparationPage.Items))
	for _, item := range preparationPage.Items {
		requirementsByID[item.ID] = item
	}
	completedRequirement := requirementsByID[requirement.ID]
	concurrentResult := requirementsByID[concurrentRequirement.ID]
	unclaimedResult := requirementsByID[unclaimedRequirement.ID]
	if preparationPage.Total != 3 ||
		preparationPage.CompletedCount != 1 ||
		preparationPage.OpenCount != 2 ||
		len(preparationPage.Items) != 3 ||
		completedRequirement.ClaimedQuantity != 10 ||
		completedRequirement.RemainingQuantity != 0 ||
		len(completedRequirement.Assignees) != 2 ||
		concurrentResult.ClaimedQuantity != 6 ||
		concurrentResult.RemainingQuantity != 4 ||
		len(concurrentResult.Assignees) != 1 ||
		unclaimedResult.Assignees == nil ||
		len(unclaimedResult.Assignees) != 0 {
		t.Fatalf("preparation page = %#v", preparationPage)
	}

	if _, _, err := meetingService.Complete(ctx, other.User.ID, created.ID); !errors.Is(err, meeting.ErrNotFound) {
		t.Fatalf("Complete(participant) error = %v, want ErrNotFound", err)
	}
	if _, _, err := meetingService.Complete(ctx, owner.User.ID, created.ID); !errors.Is(err, meeting.ErrPreparationIncomplete) {
		t.Fatalf("Complete(incomplete preparation) error = %v, want ErrPreparationIncomplete", err)
	}

	remainingClaimerID := owner.User.ID
	if concurrentResult.Assignees[0].UserID == owner.User.ID {
		remainingClaimerID = other.User.ID
	}
	if _, err := preparationService.SetClaim(
		ctx, remainingClaimerID, created.ID, concurrentRequirement.ID,
		preparation.ClaimInput{Quantity: 4},
	); err != nil {
		t.Fatalf("SetClaim(concurrent remainder) error = %v", err)
	}
	if _, err := preparationService.SetStatus(
		ctx, owner.User.ID, created.ID, concurrentRequirement.ID,
		preparation.StatusInput{Status: preparation.StatusCompleted},
	); err != nil {
		t.Fatalf("SetRequirementStatus(concurrent completed) error = %v", err)
	}
	if _, err := preparationService.SetClaim(
		ctx, owner.User.ID, created.ID, unclaimedRequirement.ID,
		preparation.ClaimInput{Quantity: 20},
	); err != nil {
		t.Fatalf("SetClaim(unclaimed requirement) error = %v", err)
	}
	if _, err := preparationService.SetStatus(
		ctx, owner.User.ID, created.ID, unclaimedRequirement.ID,
		preparation.StatusInput{Status: preparation.StatusCompleted},
	); err != nil {
		t.Fatalf("SetRequirementStatus(unclaimed completed) error = %v", err)
	}

	completion, replayedCompletion, err := meetingService.Complete(ctx, owner.User.ID, created.ID)
	if err != nil || replayedCompletion || completion.State != "completed" {
		t.Fatalf("Complete() = (%#v, %v, %v)", completion, replayedCompletion, err)
	}
	replayedResult, replayedCompletion, err := meetingService.Complete(ctx, owner.User.ID, created.ID)
	if err != nil || !replayedCompletion || replayedResult.Version != completion.Version {
		t.Fatalf("Complete(retry) = (%#v, %v, %v), want replay", replayedResult, replayedCompletion, err)
	}
	if _, err := preparationService.SetClaim(
		ctx, owner.User.ID, created.ID, unclaimedRequirement.ID,
		preparation.ClaimInput{Quantity: 0},
	); !errors.Is(err, preparation.ErrNotEditable) {
		t.Fatalf("SetClaim(after completion) error = %v, want ErrNotEditable", err)
	}
	completedMeeting, err := meetingService.Get(ctx, other.User.ID, created.ID)
	if err != nil || completedMeeting.State != "completed" {
		t.Fatalf("Get(completed) = (%#v, %v)", completedMeeting.Meeting, err)
	}
	completedPreparation, err := preparationService.List(ctx, other.User.ID, created.ID, 50, 0)
	if err != nil || completedPreparation.Total != 3 || completedPreparation.CompletedCount != 3 {
		t.Fatalf("ListPreparation(completed) = (%#v, %v)", completedPreparation, err)
	}
	completedPlanVotes, err := decisionService.List(ctx, other.User.ID, created.ID, 50, 0)
	if err != nil || completedPlanVotes.HistoryTotal != 5 {
		t.Fatalf("ListPlanVotes(completed) = (%#v, %v)", completedPlanVotes, err)
	}

	cancelCandidate, replayedCancelCandidate, err := meetingService.Create(
		ctx,
		owner.User.ID,
		"integration-cancel-create",
		meeting.CreateInput{
			Title: "Cancelled meeting", Description: "Preserve the group record", Timezone: "Asia/Novosibirsk",
		},
	)
	if err != nil || replayedCancelCandidate {
		t.Fatalf("Create(cancel candidate) = (%#v, %v, %v)", cancelCandidate, replayedCancelCandidate, err)
	}
	cancelPlanIDs := make([]uuid.UUID, 0, 2)
	for index, title := range []string{"Cancelled plan A", "Cancelled plan B"} {
		option, _, err := meetingService.AddPlanOption(
			ctx, owner.User.ID, cancelCandidate.ID,
			fmt.Sprintf("cancel-plan-%d", index),
			meeting.AddPlanOptionInput{Title: title},
		)
		if err != nil {
			t.Fatalf("AddPlanOption(cancel candidate %d) error = %v", index, err)
		}
		cancelPlanIDs = append(cancelPlanIDs, option.ID)
	}
	cancelTimeIDs := make([]uuid.UUID, 0, 2)
	for index := range 2 {
		option, _, err := meetingService.AddTimeOption(
			ctx, owner.User.ID, cancelCandidate.ID,
			fmt.Sprintf("cancel-time-%d", index),
			meeting.AddTimeOptionInput{
				StartsAt: start.Add(time.Duration(index+5) * 24 * time.Hour),
				EndsAt:   timePointer(start.Add(time.Duration(index+5)*24*time.Hour + 2*time.Hour)),
			},
		)
		if err != nil {
			t.Fatalf("AddTimeOption(cancel candidate %d) error = %v", index, err)
		}
		cancelTimeIDs = append(cancelTimeIDs, option.ID)
	}
	cancelPoll, _, err := pollService.Create(
		ctx, owner.User.ID, cancelCandidate.ID, "cancel-poll",
		poll.CreateInput{
			Question: "Keep this answer?", ResponseMode: "single",
			AllowRevote: true, Options: []string{"Yes", "No"},
		},
	)
	if err != nil {
		t.Fatalf("CreatePoll(cancel candidate) error = %v", err)
	}
	cancelSecret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x73}, 32))
	if _, _, err := meetingService.CreateInvitation(
		ctx, owner.User.ID, cancelCandidate.ID, "cancel-invitation",
		meeting.CreateInvitationInput{Secret: cancelSecret},
	); err != nil {
		t.Fatalf("CreateInvitation(cancel candidate) error = %v", err)
	}
	if _, _, err := meetingService.JoinInvitation(
		ctx, other.User.ID, meeting.JoinInvitationInput{Token: cancelSecret},
	); err != nil {
		t.Fatalf("JoinInvitation(cancel candidate) error = %v", err)
	}
	if _, err := decisionService.Vote(
		ctx, other.User.ID, cancelCandidate.ID,
		decision.VoteInput{PlanOptionID: &cancelPlanIDs[0]},
	); err != nil {
		t.Fatalf("VotePlan(cancel candidate) error = %v", err)
	}
	if _, err := pollService.Vote(
		ctx, other.User.ID, cancelPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{cancelPoll.Options[0].ID}},
	); err != nil {
		t.Fatalf("VotePoll(cancel candidate) error = %v", err)
	}
	if changed, err := noteService.Upsert(
		ctx, other.User.ID, cancelCandidate.ID,
		note.UpsertInput{Text: "Keep this detail"},
	); err != nil || !changed {
		t.Fatalf("UpsertNote(cancel candidate) = (changed %v, error %v)", changed, err)
	}
	if _, _, err := meetingService.Cancel(
		ctx, other.User.ID, cancelCandidate.ID,
	); !errors.Is(err, meeting.ErrNotFound) {
		t.Fatalf("Cancel(participant) error = %v, want ErrNotFound", err)
	}
	cancellation, replayedCancellation, err := meetingService.Cancel(
		ctx, owner.User.ID, cancelCandidate.ID,
	)
	if err != nil || replayedCancellation || cancellation.State != "cancelled" {
		t.Fatalf("Cancel() = (%#v, %v, %v)", cancellation, replayedCancellation, err)
	}
	replayedCancellationResult, replayedCancellation, err := meetingService.Cancel(
		ctx, owner.User.ID, cancelCandidate.ID,
	)
	if err != nil || !replayedCancellation ||
		replayedCancellationResult.Version != cancellation.Version {
		t.Fatalf(
			"Cancel(retry) = (%#v, %v, %v), want replay",
			replayedCancellationResult, replayedCancellation, err,
		)
	}
	if _, _, err := meetingService.JoinInvitation(
		ctx, other.User.ID, meeting.JoinInvitationInput{Token: cancelSecret},
	); !errors.Is(err, meeting.ErrInvitationInvalid) {
		t.Fatalf("JoinInvitation(cancelled) error = %v, want ErrInvitationInvalid", err)
	}
	if _, err := decisionService.Vote(
		ctx, other.User.ID, cancelCandidate.ID,
		decision.VoteInput{PlanOptionID: &cancelPlanIDs[1]},
	); !errors.Is(err, decision.ErrNotEditable) {
		t.Fatalf("VotePlan(cancelled) error = %v, want ErrNotEditable", err)
	}
	if _, err := pollService.Vote(
		ctx, other.User.ID, cancelPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{cancelPoll.Options[1].ID}},
	); !errors.Is(err, poll.ErrNotEditable) {
		t.Fatalf("VotePoll(cancelled) error = %v, want ErrNotEditable", err)
	}
	if _, err := availabilityService.Respond(
		ctx, other.User.ID, cancelTimeIDs[0],
		availability.RespondInput{Status: availability.StatusPreferred},
	); !errors.Is(err, availability.ErrNotEditable) {
		t.Fatalf("RespondAvailability(cancelled) error = %v, want ErrNotEditable", err)
	}
	cancelledMeeting, err := meetingService.Get(ctx, other.User.ID, cancelCandidate.ID)
	if err != nil || cancelledMeeting.State != "cancelled" {
		t.Fatalf("Get(cancelled) = (%#v, %v)", cancelledMeeting.Meeting, err)
	}
	cancelledPlanVotes, err := decisionService.List(
		ctx, other.User.ID, cancelCandidate.ID, 50, 0,
	)
	if err != nil || cancelledPlanVotes.HistoryTotal != 1 {
		t.Fatalf("ListPlanVotes(cancelled) = (%#v, %v)", cancelledPlanVotes, err)
	}
	cancelledPolls, err := pollService.List(ctx, other.User.ID, cancelCandidate.ID)
	if err != nil || len(cancelledPolls) != 1 ||
		!cancelledPolls[0].Options[0].SelectedByUser {
		t.Fatalf("ListPolls(cancelled) = (%#v, %v)", cancelledPolls, err)
	}
	cancelledNotes, err := noteService.List(ctx, other.User.ID, cancelCandidate.ID, 50, 0)
	if err != nil || cancelledNotes.Total != 1 || len(cancelledNotes.Items) != 1 ||
		cancelledNotes.Items[0].Text != "Keep this detail" {
		t.Fatalf("ListNotes(cancelled) = (%#v, %v)", cancelledNotes, err)
	}
	if _, err := noteService.Upsert(
		ctx, other.User.ID, cancelCandidate.ID,
		note.UpsertInput{Text: "Changed after cancellation"},
	); !errors.Is(err, note.ErrNotEditable) {
		t.Fatalf("UpsertNote(cancelled) error = %v, want ErrNotEditable", err)
	}
	if _, _, err := meetingService.Complete(
		ctx, owner.User.ID, cancelCandidate.ID,
	); !errors.Is(err, meeting.ErrNotCompletable) {
		t.Fatalf("Complete(cancelled) error = %v, want ErrNotCompletable", err)
	}
	if _, _, err := meetingService.Cancel(
		ctx, owner.User.ID, created.ID,
	); !errors.Is(err, meeting.ErrNotCancellable) {
		t.Fatalf("Cancel(completed) error = %v, want ErrNotCancellable", err)
	}

	fixedMeeting, _, err := meetingService.Create(
		ctx, owner.User.ID, "fixed-meeting-create",
		meeting.CreateInput{
			Title: "Готовый ужин", EventType: "dinner",
			CoordinationMode: "fixed", Timezone: "Asia/Novosibirsk",
			StartsAt: timePointer(start.Add(14 * 24 * time.Hour)),
		},
	)
	if err != nil {
		t.Fatalf("Create(fixed) error = %v", err)
	}
	if _, err := meetingService.Update(
		ctx, owner.User.ID, fixedMeeting.ID,
		meeting.UpdateInput{
			Title: "Готовый ужин у Анны", Description: "Нужно только ответить, кто идёт.",
			EventType: "dinner", ExpectedVersion: fixedMeeting.Version,
		},
	); err != nil {
		t.Fatalf("Update(fixed draft) error = %v", err)
	}
	fixedDraft, err := meetingService.Get(ctx, owner.User.ID, fixedMeeting.ID)
	if err != nil {
		t.Fatalf("Get(fixed draft) error = %v", err)
	}
	if len(fixedDraft.PlanOptions) != 1 || len(fixedDraft.TimeOptions) != 1 {
		t.Fatalf("fixed draft setup = (%d plans, %d times), want one of each",
			len(fixedDraft.PlanOptions), len(fixedDraft.TimeOptions))
	}
	if fixedDraft.PlanOptions[0].Title != "Готовый ужин у Анны" ||
		fixedDraft.PlanOptions[0].Description != "Нужно только ответить, кто идёт." {
		t.Fatalf("fixed draft plan metadata = %#v", fixedDraft.PlanOptions[0])
	}
	fixedPlan, fixedTime := fixedDraft.PlanOptions[0], fixedDraft.TimeOptions[0]
	if fixedTime.EndsAt != nil {
		t.Fatalf("fixed time EndsAt = %v, want nil", fixedTime.EndsAt)
	}
	fixedSecret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x82}, 32))
	if _, replayed, err := meetingService.CreateInvitation(
		ctx, owner.User.ID, fixedMeeting.ID, "fixed-invitation",
		meeting.CreateInvitationInput{Secret: fixedSecret},
	); err != nil || replayed {
		t.Fatalf("CreateInvitation(fixed) = (replayed %v, error %v)", replayed, err)
	}
	fixedDetail, err := meetingService.Get(ctx, owner.User.ID, fixedMeeting.ID)
	if err != nil {
		t.Fatalf("Get(fixed) error = %v", err)
	}
	if fixedDetail.State != "scheduled" ||
		fixedDetail.SelectedPlanOptionID == nil || *fixedDetail.SelectedPlanOptionID != fixedPlan.ID ||
		fixedDetail.SelectedTimeOptionID == nil || *fixedDetail.SelectedTimeOptionID != fixedTime.ID {
		t.Fatalf("Get(fixed) decision = %#v", fixedDetail.Meeting)
	}
	if _, joined, err := meetingService.JoinInvitation(
		ctx, other.User.ID, meeting.JoinInvitationInput{Token: fixedSecret},
	); err != nil || !joined {
		t.Fatalf("JoinInvitation(fixed) = (joined %v, error %v)", joined, err)
	}
	noteChanged, err := noteService.Upsert(
		ctx, other.User.ID, fixedMeeting.ID,
		note.UpsertInput{Text: "I can bring dessert"},
	)
	if err != nil || !noteChanged {
		t.Fatalf("UpsertNote(participant) = (changed %v, error %v)", noteChanged, err)
	}
	if noteChanged, err = noteService.Upsert(
		ctx, other.User.ID, fixedMeeting.ID,
		note.UpsertInput{Text: "I can bring dessert"},
	); err != nil || noteChanged {
		t.Fatalf("UpsertNote(retry) = (changed %v, error %v)", noteChanged, err)
	}
	if noteChanged, err = noteService.Upsert(
		ctx, owner.User.ID, fixedMeeting.ID,
		note.UpsertInput{Text: "Door code is 42"},
	); err != nil || !noteChanged {
		t.Fatalf("UpsertNote(owner) = (changed %v, error %v)", noteChanged, err)
	}
	notePage, err := noteService.List(ctx, other.User.ID, fixedMeeting.ID, 50, 0)
	if err != nil || notePage.Total != 2 || len(notePage.Items) != 2 {
		t.Fatalf("ListNotes(fixed) = (%#v, %v)", notePage, err)
	}
	notesByUser := make(map[uuid.UUID]string, len(notePage.Items))
	for _, item := range notePage.Items {
		notesByUser[item.UserID] = item.Text
	}
	if notesByUser[other.User.ID] != "I can bring dessert" || notesByUser[owner.User.ID] != "Door code is 42" {
		t.Fatalf("ListNotes(fixed) texts = %#v", notesByUser)
	}
	if noteChanged, err = noteService.Delete(ctx, owner.User.ID, fixedMeeting.ID); err != nil || !noteChanged {
		t.Fatalf("DeleteNote(owner) = (changed %v, error %v)", noteChanged, err)
	}
	fixedPoll, replayedFixedPoll, err := pollService.Create(
		ctx, other.User.ID, fixedMeeting.ID, "fixed-meeting-poll",
		poll.CreateInput{
			Question:     "Bring dessert?",
			ResponseMode: "single",
			IsAnonymous:  false,
			AllowRevote:  true,
			Options:      []string{"Yes", "No"},
		},
	)
	if err != nil || replayedFixedPoll {
		t.Fatalf("CreatePoll(fixed participant) = (%#v, %v, %v)", fixedPoll, replayedFixedPoll, err)
	}
	if fixedPoll.CreatedByUserID != other.User.ID || !fixedPoll.CanManage || !fixedPoll.AcceptingAnswers {
		t.Fatalf("CreatePoll(fixed participant) permissions = %#v", fixedPoll)
	}
	if changed, err := pollService.Vote(
		ctx, owner.User.ID, fixedPoll.ID,
		poll.VoteInput{OptionIDs: []uuid.UUID{fixedPoll.Options[0].ID}},
	); err != nil || !changed {
		t.Fatalf("VotePoll(fixed) = (changed %v, error %v)", changed, err)
	}
	if changed, err := attendanceService.Respond(
		ctx, owner.User.ID, fixedMeeting.ID,
		attendance.RespondInput{Status: attendance.StatusGoing},
	); err != nil || !changed {
		t.Fatalf("RespondAttendance(owner) = (changed %v, error %v)", changed, err)
	}
	if changed, err := attendanceService.Respond(
		ctx, other.User.ID, fixedMeeting.ID,
		attendance.RespondInput{Status: attendance.StatusMaybe},
	); err != nil || !changed {
		t.Fatalf("RespondAttendance(other) = (changed %v, error %v)", changed, err)
	}
	if changed, err := attendanceService.Respond(
		ctx, other.User.ID, fixedMeeting.ID,
		attendance.RespondInput{Status: attendance.StatusMaybe},
	); err != nil || changed {
		t.Fatalf("RespondAttendance(no-op) = (changed %v, error %v)", changed, err)
	}
	attendanceView, err := attendanceService.Get(ctx, other.User.ID, fixedMeeting.ID, 50, 0)
	if err != nil {
		t.Fatalf("GetAttendance() error = %v", err)
	}
	if attendanceView.ParticipantCount != 2 || attendanceView.GoingCount != 1 ||
		attendanceView.MaybeCount != 1 || attendanceView.NotGoingCount != 0 || attendanceView.UnansweredCount != 0 ||
		attendanceView.MyStatus != attendance.StatusMaybe {
		t.Fatalf("GetAttendance() = %#v", attendanceView)
	}
	fixedBeforeEdit, err := meetingService.Get(ctx, owner.User.ID, fixedMeeting.ID)
	if err != nil {
		t.Fatalf("Get(fixed before scheduled edit) error = %v", err)
	}
	updatedFixedStart := start.Add(21 * 24 * time.Hour)
	updatedFixedEnd := updatedFixedStart.Add(3*time.Hour + 15*time.Minute)
	updatedFixedLocation := "У Димы"
	if _, err := meetingService.Update(ctx, other.User.ID, fixedMeeting.ID, meeting.UpdateInput{
		Title: "Чужая правка", EventType: "other", StartsAt: &updatedFixedStart,
		ExpectedVersion: fixedBeforeEdit.Version,
	}); !errors.Is(err, meeting.ErrNotFound) {
		t.Fatalf("Update(fixed scheduled participant) error = %v, want ErrNotFound", err)
	}
	updatedFixed, err := meetingService.Update(ctx, owner.User.ID, fixedMeeting.ID, meeting.UpdateInput{
		Title: "Ужин перенесён", Description: "Новые детали увидит вся группа", EventType: "other",
		LocationName: &updatedFixedLocation, StartsAt: &updatedFixedStart, EndsAt: &updatedFixedEnd,
		ExpectedVersion: fixedBeforeEdit.Version,
	})
	if err != nil || updatedFixed.State != "scheduled" || updatedFixed.Version != fixedBeforeEdit.Version+1 {
		t.Fatalf("Update(fixed scheduled) = (%#v, %v)", updatedFixed, err)
	}
	fixedAfterEdit, err := meetingService.Get(ctx, other.User.ID, fixedMeeting.ID)
	if err != nil || fixedAfterEdit.Title != "Ужин перенесён" ||
		len(fixedAfterEdit.PlanOptions) != 1 || fixedAfterEdit.PlanOptions[0].Title != "Ужин перенесён" ||
		len(fixedAfterEdit.TimeOptions) != 1 || !fixedAfterEdit.TimeOptions[0].StartsAt.Equal(updatedFixedStart) ||
		fixedAfterEdit.TimeOptions[0].EndsAt == nil || !fixedAfterEdit.TimeOptions[0].EndsAt.Equal(updatedFixedEnd) {
		t.Fatalf("Get(fixed after scheduled edit) = (%#v, %v)", fixedAfterEdit, err)
	}
	attendanceAfterEdit, err := attendanceService.Get(ctx, other.User.ID, fixedMeeting.ID, 50, 0)
	if err != nil || attendanceAfterEdit.GoingCount != 1 || attendanceAfterEdit.MaybeCount != 1 ||
		attendanceAfterEdit.MyStatus != attendance.StatusMaybe {
		t.Fatalf("GetAttendance(after fixed edit) = (%#v, %v)", attendanceAfterEdit, err)
	}
	fixedPhotoMutation, err := mediaService.PutMeetingPhoto(
		ctx, owner.User.ID, fixedMeeting.ID, fixedAfterEdit.Version, "image/png", meetingPhoto,
	)
	if err != nil || !fixedPhotoMutation.Changed || fixedPhotoMutation.Version != fixedAfterEdit.Version+1 {
		t.Fatalf("PutMeetingPhoto(fixed scheduled) = (%#v, %v)", fixedPhotoMutation, err)
	}

	directInvitee, err := authService.Register(ctx, auth.RegisterInput{
		Email: "invitee@example.test", Password: "safe direct invite password", DisplayName: "Ира", Nickname: "ira_invitee",
	})
	if err != nil {
		t.Fatalf("Register(direct invitee) error = %v", err)
	}
	if _, err := friendshipService.Send(ctx, owner.User.ID, directInvitee.User.ID); err != nil {
		t.Fatalf("SendFriendRequest(direct invitee) error = %v", err)
	}
	directInviteeFriends, err := friendshipService.Overview(ctx, directInvitee.User.ID, 50, 0)
	if err != nil || len(directInviteeFriends.Incoming.Items) != 1 {
		t.Fatalf("Overview(direct invitee) = (%#v, %v)", directInviteeFriends, err)
	}
	if _, err := friendshipService.Accept(ctx, directInvitee.User.ID, directInviteeFriends.Incoming.Items[0].RequestID); err != nil {
		t.Fatalf("AcceptFriendRequest(direct invitee) error = %v", err)
	}
	candidates, err := meetingInviteService.Candidates(ctx, owner.User.ID, fixedMeeting.ID, 50, 0)
	if err != nil || candidates.Total < 2 {
		t.Fatalf("MeetingInviteCandidates() = (%#v, %v)", candidates, err)
	}
	sentInvites, err := meetingInviteService.Send(
		ctx, owner.User.ID, fixedMeeting.ID, []uuid.UUID{directInvitee.User.ID, directInvitee.User.ID},
	)
	if err != nil || sentInvites.ChangedCount != 1 {
		t.Fatalf("SendMeetingInvites() = (%#v, %v)", sentInvites, err)
	}
	if replay, err := meetingInviteService.Send(
		ctx, owner.User.ID, fixedMeeting.ID, []uuid.UUID{directInvitee.User.ID},
	); err != nil || replay.ChangedCount != 0 {
		t.Fatalf("SendMeetingInvites(retry) = (%#v, %v)", replay, err)
	}
	incomingInvites, err := meetingInviteService.Incoming(ctx, directInvitee.User.ID, 50, 0)
	if err != nil || incomingInvites.Total != 1 || len(incomingInvites.Items) != 1 {
		t.Fatalf("IncomingMeetingInvites() = (%#v, %v)", incomingInvites, err)
	}
	acceptedInvite, err := meetingInviteService.Accept(
		ctx, directInvitee.User.ID, incomingInvites.Items[0].ID,
	)
	if err != nil || !acceptedInvite.Changed || !acceptedInvite.Joined || acceptedInvite.MeetingID != fixedMeeting.ID {
		t.Fatalf("AcceptMeetingInvite() = (%#v, %v)", acceptedInvite, err)
	}
	if _, err := meetingService.Get(ctx, directInvitee.User.ID, fixedMeeting.ID); err != nil {
		t.Fatalf("Get(after direct invitation acceptance) error = %v", err)
	}
	acceptedReplay, err := meetingInviteService.Accept(ctx, directInvitee.User.ID, incomingInvites.Items[0].ID)
	if err != nil || acceptedReplay.Changed || acceptedReplay.Joined {
		t.Fatalf("AcceptMeetingInvite(retry) = (%#v, %v)", acceptedReplay, err)
	}

	if _, err := uuid.Parse(created.ID.String()); err != nil {
		t.Fatalf("created ID is not a UUID: %v", err)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
