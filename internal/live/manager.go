package live

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrMeetingLimit    = errors.New("live meeting subscription limit reached")
	ErrSubscriberLimit = errors.New("live subscriber limit reached")
)

type Event struct {
	Version int64 `json:"version"`
}

type VersionSource interface {
	Versions(context.Context, []uuid.UUID) (map[uuid.UUID]int64, error)
}

type Observer interface {
	LiveConnectionOpened()
	LiveConnectionClosed()
	LiveUpdatePublished()
	LivePollFailed()
}

type Options struct {
	PollInterval        time.Duration
	PollTimeout         time.Duration
	MaxMeetings         int
	MaxSubscribersPerID int
}

type Manager struct {
	source   VersionSource
	observer Observer
	logger   *slog.Logger
	options  Options

	mu       sync.Mutex
	nextID   uint64
	meetings map[uuid.UUID]*meetingSubscribers
}

type meetingSubscribers struct {
	version     int64
	subscribers map[uint64]chan Event
}

type Subscription struct {
	Events <-chan Event
	close  func()
	once   sync.Once
}

func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(s.close)
}

func NewManager(
	source VersionSource,
	observer Observer,
	logger *slog.Logger,
	options Options,
) *Manager {
	if options.PollInterval <= 0 {
		options.PollInterval = 2 * time.Second
	}
	if options.PollTimeout <= 0 {
		options.PollTimeout = 1500 * time.Millisecond
	}
	if options.MaxMeetings <= 0 {
		options.MaxMeetings = 1000
	}
	if options.MaxSubscribersPerID <= 0 {
		options.MaxSubscribersPerID = 100
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		source:   source,
		observer: observer,
		logger:   logger,
		options:  options,
		meetings: make(map[uuid.UUID]*meetingSubscribers),
	}
}

func (m *Manager) Subscribe(meetingID uuid.UUID, initialVersion int64) (*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	group, ok := m.meetings[meetingID]
	if !ok {
		if len(m.meetings) >= m.options.MaxMeetings {
			return nil, ErrMeetingLimit
		}
		group = &meetingSubscribers{
			version:     initialVersion,
			subscribers: make(map[uint64]chan Event),
		}
		m.meetings[meetingID] = group
	} else if initialVersion > group.version {
		group.version = initialVersion
		for _, events := range group.subscribers {
			publishLatest(events, Event{Version: initialVersion})
			if m.observer != nil {
				m.observer.LiveUpdatePublished()
			}
		}
	}
	if len(group.subscribers) >= m.options.MaxSubscribersPerID {
		return nil, ErrSubscriberLimit
	}

	m.nextID++
	subscriptionID := m.nextID
	events := make(chan Event, 1)
	group.subscribers[subscriptionID] = events
	if m.observer != nil {
		m.observer.LiveConnectionOpened()
	}

	return &Subscription{
		Events: events,
		close: func() {
			m.unsubscribe(meetingID, subscriptionID)
		},
	}, nil
}

func (m *Manager) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.options.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *Manager) poll(parent context.Context) {
	meetingIDs := m.snapshotMeetingIDs()
	if len(meetingIDs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(parent, m.options.PollTimeout)
	defer cancel()
	versions, err := m.source.Versions(ctx, meetingIDs)
	if err != nil {
		if m.observer != nil {
			m.observer.LivePollFailed()
		}
		m.logger.WarnContext(parent, "live meeting version poll failed", "error", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for meetingID, version := range versions {
		group, ok := m.meetings[meetingID]
		if !ok || version <= group.version {
			continue
		}
		group.version = version
		for _, events := range group.subscribers {
			publishLatest(events, Event{Version: version})
			if m.observer != nil {
				m.observer.LiveUpdatePublished()
			}
		}
	}
}

func (m *Manager) snapshotMeetingIDs() []uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]uuid.UUID, 0, len(m.meetings))
	for meetingID := range m.meetings {
		result = append(result, meetingID)
	}
	return result
}

func (m *Manager) unsubscribe(meetingID uuid.UUID, subscriptionID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	group, ok := m.meetings[meetingID]
	if !ok {
		return
	}
	if _, ok := group.subscribers[subscriptionID]; !ok {
		return
	}
	delete(group.subscribers, subscriptionID)
	if len(group.subscribers) == 0 {
		delete(m.meetings, meetingID)
	}
	if m.observer != nil {
		m.observer.LiveConnectionClosed()
	}
}

func publishLatest(events chan Event, event Event) {
	select {
	case events <- event:
		return
	default:
	}
	select {
	case <-events:
	default:
	}
	select {
	case events <- event:
	default:
	}
}
