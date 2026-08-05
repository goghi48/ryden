package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	requests             *prometheus.CounterVec
	duration             *prometheus.HistogramVec
	inFlight             prometheus.Gauge
	meetings             prometheus.Counter
	meetingEdits         prometheus.Counter
	setupOptionEdits     *prometheus.CounterVec
	photoChanges         *prometheus.CounterVec
	friendshipChanges    *prometheus.CounterVec
	meetingInviteChanges *prometheus.CounterVec
	invitations          prometheus.Counter
	participants         prometheus.Counter
	polls                prometheus.Counter
	pollVotes            prometheus.Counter
	pollClosures         *prometheus.CounterVec
	availability         prometheus.Counter
	attendance           prometheus.Counter
	noteChanges          *prometheus.CounterVec
	planVotes            prometheus.Counter
	decisions            prometheus.Counter
	requirements         prometheus.Counter
	requirementEdits     prometheus.Counter
	requirementDeletes   prometheus.Counter
	claims               prometheus.Counter
	prepStatuses         prometheus.Counter
	completions          prometheus.Counter
	cancellations        prometheus.Counter
	liveConnections      prometheus.Gauge
	liveUpdates          prometheus.Counter
	livePollErrors       prometheus.Counter
}

func NewMetrics(registry *prometheus.Registry) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_http_requests_total",
			Help: "Total HTTP requests by method, route group, and status class.",
		}, []string{"method", "route_group", "status_class"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ryden_http_request_duration_seconds",
			Help:    "HTTP request duration by method and route group.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route_group"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ryden_http_requests_in_flight",
			Help: "Current HTTP requests in flight.",
		}),
		meetings: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meetings_created_total",
			Help: "Meetings successfully created.",
		}),
		meetingEdits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meetings_updated_total",
			Help: "Draft meeting metadata updates successfully applied.",
		}),
		setupOptionEdits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_meeting_setup_options_updated_total",
			Help: "Draft meeting plan and time option updates successfully applied.",
		}, []string{"option_type"}),
		photoChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_meeting_photos_changed_total",
			Help: "Effective meeting and plan-option photo uploads, replacements, and removals.",
		}, []string{"scope"}),
		friendshipChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_friendship_changes_total",
			Help: "Effective friend request and friendship changes by bounded action.",
		}, []string{"action"}),
		meetingInviteChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_direct_meeting_invitation_changes_total",
			Help: "Effective direct meeting invitation changes by bounded action.",
		}, []string{"action"}),
		invitations: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meeting_invitations_created_total",
			Help: "Meeting invitations successfully created.",
		}),
		participants: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meeting_participants_joined_total",
			Help: "Participants newly joined through invitations.",
		}),
		polls: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_polls_created_total",
			Help: "Generic polls successfully created.",
		}),
		pollVotes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_poll_votes_submitted_total",
			Help: "Effective generic poll answer changes, including replacements and retractions.",
		}),
		pollClosures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_polls_closed_total",
			Help: "Generic polls closed by outcome mode.",
		}, []string{"outcome"}),
		availability: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_availability_responses_submitted_total",
			Help: "Availability responses created, changed, or cleared.",
		}),
		attendance: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_attendance_responses_changed_total",
			Help: "Effective fixed-meeting attendance response changes and retractions.",
		}),
		noteChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ryden_meeting_notes_changed_total",
			Help: "Effective meeting note changes by bounded action.",
		}, []string{"action"}),
		planVotes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_plan_votes_changed_total",
			Help: "Effective plan vote casts, replacements, and retractions.",
		}),
		decisions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meeting_decisions_finalized_total",
			Help: "Meetings successfully finalized with a plan and compatible time.",
		}),
		requirements: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_requirements_created_total",
			Help: "Preparation requirements successfully created.",
		}),
		requirementEdits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_requirements_updated_total",
			Help: "Effective preparation requirement name or quantity updates.",
		}),
		requirementDeletes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_requirements_deleted_total",
			Help: "Unclaimed preparation requirements successfully deleted.",
		}),
		claims: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_requirement_claims_changed_total",
			Help: "Effective preparation claim creations, replacements, and retractions.",
		}),
		prepStatuses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_requirement_status_changes_total",
			Help: "Effective preparation requirement completion and reopen changes.",
		}),
		completions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meetings_completed_total",
			Help: "Meetings successfully completed by their owners.",
		}),
		cancellations: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_meetings_cancelled_total",
			Help: "Meetings successfully cancelled by their owners.",
		}),
		liveConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ryden_live_connections",
			Help: "Current authorized meeting update streams.",
		}),
		liveUpdates: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_live_updates_published_total",
			Help: "Meeting version updates published to active streams.",
		}),
		livePollErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ryden_live_version_poll_errors_total",
			Help: "Failed PostgreSQL meeting version polling attempts.",
		}),
	}
	registry.MustRegister(
		m.requests, m.duration, m.inFlight, m.meetings, m.meetingEdits, m.setupOptionEdits, m.photoChanges, m.friendshipChanges, m.meetingInviteChanges, m.invitations,
		m.participants, m.polls, m.pollVotes, m.pollClosures, m.availability, m.attendance, m.noteChanges, m.planVotes, m.decisions,
		m.requirements, m.requirementEdits, m.requirementDeletes,
		m.claims, m.prepStatuses, m.completions, m.cancellations,
		m.liveConnections, m.liveUpdates, m.livePollErrors,
	)
	return m
}

func (m *Metrics) MeetingCreated() {
	m.meetings.Inc()
}

func (m *Metrics) MeetingUpdated() {
	m.meetingEdits.Inc()
}

func (m *Metrics) SetupOptionUpdated(optionType string) {
	m.setupOptionEdits.WithLabelValues(optionType).Inc()
}

func (m *Metrics) PhotoChanged(scope string) {
	m.photoChanges.WithLabelValues(scope).Inc()
}

func (m *Metrics) FriendshipChanged(action string) {
	m.friendshipChanges.WithLabelValues(action).Inc()
}

func (m *Metrics) MeetingInviteChanged(action string, count int) {
	m.meetingInviteChanges.WithLabelValues(action).Add(float64(count))
}

func (m *Metrics) InvitationCreated() {
	m.invitations.Inc()
}

func (m *Metrics) ParticipantJoined() {
	m.participants.Inc()
}

func (m *Metrics) PollCreated() {
	m.polls.Inc()
}

func (m *Metrics) PollVoteSubmitted() {
	m.pollVotes.Inc()
}

func (m *Metrics) PollClosed(withDecision bool) {
	outcome := "without_decision"
	if withDecision {
		outcome = "decision"
	}
	m.pollClosures.WithLabelValues(outcome).Inc()
}

func (m *Metrics) AvailabilityResponseSubmitted() {
	m.availability.Inc()
}

func (m *Metrics) AttendanceResponseChanged() {
	m.attendance.Inc()
}

func (m *Metrics) MeetingNoteChanged(action string) {
	m.noteChanges.WithLabelValues(action).Inc()
}

func (m *Metrics) PlanVoteChanged() {
	m.planVotes.Inc()
}

func (m *Metrics) DecisionFinalized() {
	m.decisions.Inc()
}

func (m *Metrics) RequirementCreated() {
	m.requirements.Inc()
}

func (m *Metrics) RequirementUpdated() {
	m.requirementEdits.Inc()
}

func (m *Metrics) RequirementDeleted() {
	m.requirementDeletes.Inc()
}

func (m *Metrics) RequirementClaimChanged() {
	m.claims.Inc()
}

func (m *Metrics) RequirementStatusChanged() {
	m.prepStatuses.Inc()
}

func (m *Metrics) MeetingCompleted() {
	m.completions.Inc()
}

func (m *Metrics) MeetingCancelled() {
	m.cancellations.Inc()
}

func (m *Metrics) LiveConnectionOpened() {
	m.liveConnections.Inc()
}

func (m *Metrics) LiveConnectionClosed() {
	m.liveConnections.Dec()
}

func (m *Metrics) LiveUpdatePublished() {
	m.liveUpdates.Inc()
}

func (m *Metrics) LivePollFailed() {
	m.livePollErrors.Inc()
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		group := routeGroup(r.URL.Path)
		statusClass := strconv.Itoa(recorder.status/100) + "xx"
		m.requests.WithLabelValues(r.Method, group, statusClass).Inc()
		m.duration.WithLabelValues(r.Method, group).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func routeGroup(path string) string {
	switch {
	case path == "/livez" || path == "/readyz" || path == "/startupz":
		return "health"
	case path == "/metrics":
		return "metrics"
	case len(path) >= len("/api/v1/auth/") && path[:len("/api/v1/auth/")] == "/api/v1/auth/":
		return "auth"
	case path == "/api/v1/me":
		return "profile"
	case path == "/api/v1/me/avatar":
		return "profile"
	case len(path) >= len("/api/v1/me/meeting-invitations") && path[:len("/api/v1/me/meeting-invitations")] == "/api/v1/me/meeting-invitations":
		return "meeting_invitations"
	case len(path) >= len("/api/v1/users/") && path[:len("/api/v1/users/")] == "/api/v1/users/" && len(path) >= len("/avatar") && path[len(path)-len("/avatar"):] == "/avatar":
		return "profile"
	case len(path) >= len("/api/v1/friends") && path[:len("/api/v1/friends")] == "/api/v1/friends":
		return "friends"
	case len(path) >= len("/api/v1/friend-requests") && path[:len("/api/v1/friend-requests")] == "/api/v1/friend-requests":
		return "friends"
	case path == "/api/v1/users/search":
		return "friends"
	case len(path) >= len("/api/v1/meetings") && path[:len("/api/v1/meetings")] == "/api/v1/meetings":
		return "meetings"
	case len(path) >= len("/api/v1/time-options") && path[:len("/api/v1/time-options")] == "/api/v1/time-options":
		return "meetings"
	default:
		return "other"
	}
}
