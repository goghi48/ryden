package live

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeVersionSource struct {
	mu       sync.Mutex
	versions map[uuid.UUID]int64
}

func (f *fakeVersionSource) Versions(
	_ context.Context,
	meetingIDs []uuid.UUID,
) (map[uuid.UUID]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make(map[uuid.UUID]int64, len(meetingIDs))
	for _, meetingID := range meetingIDs {
		if version, ok := f.versions[meetingID]; ok {
			result[meetingID] = version
		}
	}
	return result, nil
}

func (f *fakeVersionSource) set(meetingID uuid.UUID, version int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions[meetingID] = version
}

func TestManagerPublishesNewMeetingVersion(t *testing.T) {
	meetingID := uuid.New()
	source := &fakeVersionSource{versions: map[uuid.UUID]int64{meetingID: 1}}
	manager := newTestManager(source, Options{
		PollInterval:        5 * time.Millisecond,
		PollTimeout:         time.Second,
		MaxMeetings:         2,
		MaxSubscribersPerID: 2,
	})
	subscription, err := manager.Subscribe(meetingID, 1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()

	source.set(meetingID, 3)
	select {
	case event := <-subscription.Events:
		if event.Version != 3 {
			t.Fatalf("event version = %d, want 3", event.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live update")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("manager did not stop")
	}
}

func TestManagerEnforcesLimitsAndReleasesSubscription(t *testing.T) {
	manager := newTestManager(
		&fakeVersionSource{versions: map[uuid.UUID]int64{}},
		Options{MaxMeetings: 1, MaxSubscribersPerID: 1},
	)
	firstID := uuid.New()
	first, err := manager.Subscribe(firstID, 1)
	if err != nil {
		t.Fatalf("first subscribe: %v", err)
	}
	if _, err := manager.Subscribe(firstID, 1); err != ErrSubscriberLimit {
		t.Fatalf("second subscriber error = %v, want %v", err, ErrSubscriberLimit)
	}
	if _, err := manager.Subscribe(uuid.New(), 1); err != ErrMeetingLimit {
		t.Fatalf("second meeting error = %v, want %v", err, ErrMeetingLimit)
	}

	first.Close()
	first.Close()
	replacement, err := manager.Subscribe(uuid.New(), 1)
	if err != nil {
		t.Fatalf("replacement subscribe: %v", err)
	}
	replacement.Close()
}

func TestNewerSubscriberAdvancesExistingSubscribers(t *testing.T) {
	meetingID := uuid.New()
	manager := newTestManager(
		&fakeVersionSource{versions: map[uuid.UUID]int64{meetingID: 3}},
		Options{MaxMeetings: 1, MaxSubscribersPerID: 2},
	)
	first, err := manager.Subscribe(meetingID, 1)
	if err != nil {
		t.Fatalf("first subscribe: %v", err)
	}
	defer first.Close()
	second, err := manager.Subscribe(meetingID, 3)
	if err != nil {
		t.Fatalf("second subscribe: %v", err)
	}
	defer second.Close()

	select {
	case event := <-first.Events:
		if event.Version != 3 {
			t.Fatalf("existing subscriber version = %d, want 3", event.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("existing subscriber did not receive newer authorized version")
	}
	select {
	case event := <-second.Events:
		t.Fatalf("new subscriber received redundant event: %#v", event)
	default:
	}
}

func newTestManager(source VersionSource, options Options) *Manager {
	return NewManager(
		source,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		options,
	)
}
