package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/goghi48/ryden/internal/attendance"
	"github.com/goghi48/ryden/internal/auth"
	"github.com/goghi48/ryden/internal/availability"
	"github.com/goghi48/ryden/internal/calendar"
	"github.com/goghi48/ryden/internal/decision"
	"github.com/goghi48/ryden/internal/friendship"
	"github.com/goghi48/ryden/internal/live"
	"github.com/goghi48/ryden/internal/media"
	"github.com/goghi48/ryden/internal/meeting"
	"github.com/goghi48/ryden/internal/meetinginvite"
	"github.com/goghi48/ryden/internal/note"
	"github.com/goghi48/ryden/internal/observability"
	"github.com/goghi48/ryden/internal/poll"
	"github.com/goghi48/ryden/internal/preparation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	refreshCookieName = "ryden_refresh"
	maxBodyBytes      = 1 << 20
	requestTimeout    = 10 * time.Second
)

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	requestIDKey contextKey = "request_id"
)

type Server struct {
	mux            *http.ServeMux
	auth           *auth.Service
	friends        *friendship.Service
	meetingInvites *meetinginvite.Service
	meetings       *meeting.Service
	polls          *poll.Service
	availability   *availability.Service
	attendance     *attendance.Service
	notes          *note.Service
	calendar       *calendar.Service
	decisions      *decision.Service
	media          *media.Service
	preparation    *preparation.Service
	live           *live.Manager
	tokens         *auth.TokenManager
	pool           *pgxpool.Pool
	metrics        *observability.Metrics
	logger         *slog.Logger
	allowedOrigin  string
	cookieSecure   bool
}

type Options struct {
	Auth           *auth.Service
	Friends        *friendship.Service
	MeetingInvites *meetinginvite.Service
	Meetings       *meeting.Service
	Polls          *poll.Service
	Availability   *availability.Service
	Attendance     *attendance.Service
	Notes          *note.Service
	Calendar       *calendar.Service
	Decisions      *decision.Service
	Media          *media.Service
	Preparation    *preparation.Service
	Live           *live.Manager
	Tokens         *auth.TokenManager
	Pool           *pgxpool.Pool
	Metrics        *observability.Metrics
	Logger         *slog.Logger
	MetricsHandler http.Handler
	AllowedOrigin  string
	CookieSecure   bool
}

func NewServer(options Options) http.Handler {
	server := &Server{
		mux:            http.NewServeMux(),
		auth:           options.Auth,
		friends:        options.Friends,
		meetingInvites: options.MeetingInvites,
		meetings:       options.Meetings,
		polls:          options.Polls,
		availability:   options.Availability,
		attendance:     options.Attendance,
		notes:          options.Notes,
		calendar:       options.Calendar,
		decisions:      options.Decisions,
		media:          options.Media,
		preparation:    options.Preparation,
		live:           options.Live,
		tokens:         options.Tokens,
		pool:           options.Pool,
		metrics:        options.Metrics,
		logger:         options.Logger,
		allowedOrigin:  options.AllowedOrigin,
		cookieSecure:   options.CookieSecure,
	}
	server.routes(options.MetricsHandler)

	var handler http.Handler = server.mux
	handler = server.timeout(handler)
	handler = server.securityHeaders(handler)
	handler = server.cors(handler)
	handler = options.Metrics.Middleware(handler)
	handler = server.logging(handler)
	handler = server.requestID(handler)
	handler = server.recover(handler)
	return handler
}

func (s *Server) routes(metricsHandler http.Handler) {
	s.mux.HandleFunc("GET /livez", s.handleLive)
	s.mux.HandleFunc("GET /startupz", s.handleStartup)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	if metricsHandler == nil {
		metricsHandler = promhttp.Handler()
	}
	s.mux.Handle("GET /metrics", metricsHandler)
	s.mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	s.mux.HandleFunc("POST /api/v1/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.mux.Handle("GET /api/v1/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	s.mux.Handle("PUT /api/v1/me", s.requireAuth(http.HandlerFunc(s.handleProfileUpdate)))
	s.mux.Handle("PUT /api/v1/me/avatar", s.requireAuth(http.HandlerFunc(s.handleUserAvatarPut)))
	s.mux.Handle("DELETE /api/v1/me/avatar", s.requireAuth(http.HandlerFunc(s.handleUserAvatarDelete)))
	s.mux.Handle("GET /api/v1/users/search", s.requireAuth(http.HandlerFunc(s.handleUserSearch)))
	s.mux.Handle("GET /api/v1/users/{userID}/avatar", s.requireAuth(http.HandlerFunc(s.handleUserAvatarGet)))
	s.mux.Handle("GET /api/v1/friends", s.requireAuth(http.HandlerFunc(s.handleFriendsOverview)))
	s.mux.Handle("POST /api/v1/friend-requests", s.requireAuth(http.HandlerFunc(s.handleFriendRequestSend)))
	s.mux.Handle("PUT /api/v1/friend-requests/{requestID}", s.requireAuth(http.HandlerFunc(s.handleFriendRequestAccept)))
	s.mux.Handle("DELETE /api/v1/friend-requests/{requestID}", s.requireAuth(http.HandlerFunc(s.handleFriendRequestDelete)))
	s.mux.Handle("DELETE /api/v1/friends/{friendID}", s.requireAuth(http.HandlerFunc(s.handleFriendRemove)))
	s.mux.Handle("GET /api/v1/me/meeting-invitations", s.requireAuth(http.HandlerFunc(s.handleMeetingInviteIncoming)))
	s.mux.Handle("PUT /api/v1/me/meeting-invitations/{invitationID}", s.requireAuth(http.HandlerFunc(s.handleMeetingInviteAccept)))
	s.mux.Handle("DELETE /api/v1/me/meeting-invitations/{invitationID}", s.requireAuth(http.HandlerFunc(s.handleMeetingInviteDecline)))
	s.mux.Handle("GET /api/v1/meetings", s.requireAuth(http.HandlerFunc(s.handleMeetingList)))
	s.mux.Handle("POST /api/v1/meetings", s.requireAuth(http.HandlerFunc(s.handleMeetingCreate)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}", s.requireAuth(http.HandlerFunc(s.handleMeetingGet)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}", s.requireAuth(http.HandlerFunc(s.handleMeetingUpdate)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/photo", s.requireAuth(http.HandlerFunc(s.handleMeetingPhotoGet)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/photo", s.requireAuth(http.HandlerFunc(s.handleMeetingPhotoPut)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/photo", s.requireAuth(http.HandlerFunc(s.handleMeetingPhotoDelete)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/calendar.ics", s.requireAuth(http.HandlerFunc(s.handleMeetingCalendar)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/events", s.requireAuth(http.HandlerFunc(s.handleMeetingEvents)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/plan-options", s.requireAuth(http.HandlerFunc(s.handlePlanOptionCreate)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/plan-options/{optionID}", s.requireAuth(http.HandlerFunc(s.handlePlanOptionUpdate)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/plan-options/{optionID}", s.requireAuth(http.HandlerFunc(s.handlePlanOptionDelete)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/plan-options/{optionID}/photo", s.requireAuth(http.HandlerFunc(s.handlePlanOptionPhotoGet)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/plan-options/{optionID}/photo", s.requireAuth(http.HandlerFunc(s.handlePlanOptionPhotoPut)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/plan-options/{optionID}/photo", s.requireAuth(http.HandlerFunc(s.handlePlanOptionPhotoDelete)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/time-options", s.requireAuth(http.HandlerFunc(s.handleTimeOptionCreate)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/time-options/{optionID}", s.requireAuth(http.HandlerFunc(s.handleTimeOptionUpdate)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/time-options/{optionID}", s.requireAuth(http.HandlerFunc(s.handleTimeOptionDelete)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/invitations", s.requireAuth(http.HandlerFunc(s.handleInvitationCreate)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/invitation", s.requireAuth(http.HandlerFunc(s.handleInvitationRevoke)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/friend-invitations", s.requireAuth(http.HandlerFunc(s.handleMeetingInviteCandidates)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/friend-invitations", s.requireAuth(http.HandlerFunc(s.handleMeetingInviteSend)))
	s.mux.Handle("POST /api/v1/invitations/join", s.requireAuth(http.HandlerFunc(s.handleInvitationJoin)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/polls", s.requireAuth(http.HandlerFunc(s.handlePollList)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/polls", s.requireAuth(http.HandlerFunc(s.handlePollCreate)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/polls/{pollID}", s.requireAuth(http.HandlerFunc(s.handlePollDelete)))
	s.mux.Handle("GET /api/v1/polls/{pollID}/history", s.requireAuth(http.HandlerFunc(s.handlePollHistory)))
	s.mux.Handle("PUT /api/v1/polls/{pollID}/vote", s.requireAuth(http.HandlerFunc(s.handlePollVote)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/polls/{pollID}/close", s.requireAuth(http.HandlerFunc(s.handlePollClose)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/availability", s.requireAuth(http.HandlerFunc(s.handleAvailabilityList)))
	s.mux.Handle("PUT /api/v1/time-options/{timeOptionID}/availability", s.requireAuth(http.HandlerFunc(s.handleAvailabilityRespond)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/attendance", s.requireAuth(http.HandlerFunc(s.handleAttendanceGet)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/attendance", s.requireAuth(http.HandlerFunc(s.handleAttendanceRespond)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/notes", s.requireAuth(http.HandlerFunc(s.handleNoteList)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/notes/mine", s.requireAuth(http.HandlerFunc(s.handleNoteUpsert)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/notes/mine", s.requireAuth(http.HandlerFunc(s.handleNoteDelete)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/plan-votes", s.requireAuth(http.HandlerFunc(s.handlePlanVoteList)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/plan-vote", s.requireAuth(http.HandlerFunc(s.handlePlanVote)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/decision", s.requireAuth(http.HandlerFunc(s.handleDecisionFinalize)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/complete", s.requireAuth(http.HandlerFunc(s.handleMeetingComplete)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/cancel", s.requireAuth(http.HandlerFunc(s.handleMeetingCancel)))
	s.mux.Handle("GET /api/v1/meetings/{meetingID}/requirements", s.requireAuth(http.HandlerFunc(s.handleRequirementList)))
	s.mux.Handle("POST /api/v1/meetings/{meetingID}/requirements", s.requireAuth(http.HandlerFunc(s.handleRequirementCreate)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/requirements/{requirementID}", s.requireAuth(http.HandlerFunc(s.handleRequirementUpdate)))
	s.mux.Handle("DELETE /api/v1/meetings/{meetingID}/requirements/{requirementID}", s.requireAuth(http.HandlerFunc(s.handleRequirementDelete)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/requirements/{requirementID}/claim", s.requireAuth(http.HandlerFunc(s.handleRequirementClaim)))
	s.mux.Handle("PUT /api/v1/meetings/{meetingID}/requirements/{requirementID}/status", s.requireAuth(http.HandlerFunc(s.handleRequirementStatus)))
}

func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStartup(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "not_ready", "База данных пока недоступна.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	session, err := s.auth.Register(r.Context(), auth.RegisterInput{
		Email: request.Email, Password: request.Password, DisplayName: request.DisplayName, Nickname: request.Nickname,
	})
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	s.setRefreshCookie(w, session)
	writeJSON(w, http.StatusCreated, session)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	session, err := s.auth.Login(r.Context(), auth.LoginInput{
		Email: request.Email, Password: request.Password,
	})
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	s.setRefreshCookie(w, session)
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		s.clearRefreshCookie(w)
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Сессия завершена. Войдите снова.")
		return
	}
	session, err := s.auth.Refresh(r.Context(), cookie.Value)
	if err != nil {
		s.clearRefreshCookie(w)
		s.writeAuthError(w, err)
		return
	}
	s.setRefreshCookie(w, session)
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			s.logger.ErrorContext(r.Context(), "logout failed", "error", err)
		}
	}
	s.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.auth.User(r.Context(), mustUserID(r.Context()))
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type updateProfileRequest struct {
	DisplayName string  `json:"display_name"`
	Nickname    string  `json:"nickname"`
	AvatarURL   *string `json:"avatar_url"`
}

func (s *Server) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var request updateProfileRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	user, err := s.auth.UpdateProfile(r.Context(), mustUserID(r.Context()), auth.UpdateProfileInput{
		DisplayName: request.DisplayName,
		Nickname:    request.Nickname,
		AvatarURL:   request.AvatarURL,
	})
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleUserAvatarGet(w http.ResponseWriter, r *http.Request) {
	userID, ok := parsePathUUID(w, r, "userID", "avatar_not_found", "Аватар не найден.")
	if !ok {
		return
	}
	photo, err := s.media.GetUserAvatar(r.Context(), mustUserID(r.Context()), userID)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	writePhoto(w, r, photo)
}

func (s *Server) handleUserAvatarPut(w http.ResponseWriter, r *http.Request) {
	content, ok := readPhotoBodyLimit(w, r, media.MaxAvatarBytes, "Аватар должен быть не больше 1 МБ.")
	if !ok {
		return
	}
	result, err := s.media.PutUserAvatar(
		r.Context(), mustUserID(r.Context()), r.Header.Get("Content-Type"), content,
	)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	if result.Changed {
		s.metrics.PhotoChanged("avatar")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUserAvatarDelete(w http.ResponseWriter, r *http.Request) {
	result, err := s.media.DeleteUserAvatar(r.Context(), mustUserID(r.Context()))
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	if result.Changed {
		s.metrics.PhotoChanged("avatar")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingCreate(w http.ResponseWriter, r *http.Request) {
	var input meeting.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.meetings.Create(
		r.Context(),
		mustUserID(r.Context()),
		r.Header.Get("Idempotency-Key"),
		input,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, result)
		return
	}
	s.metrics.MeetingCreated()
	w.Header().Set("Location", "/api/v1/meetings/"+result.ID.String())
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleMeetingList(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	page, err := s.meetings.List(r.Context(), mustUserID(r.Context()), limit, offset)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleMeetingGet(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	result, err := s.meetings.Get(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingCalendar(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	result, err := s.calendar.Export(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		switch {
		case errors.Is(err, meeting.ErrNotFound):
			writeProblem(w, http.StatusNotFound, "meeting_not_found", "Встреча не найдена.")
		case errors.Is(err, calendar.ErrNotAvailable):
			writeProblem(w, http.StatusConflict, "calendar_not_available", "Добавить в календарь можно только подтверждённую или завершённую встречу.")
		default:
			s.logger.ErrorContext(r.Context(), "calendar export failed", "error", err)
			writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось подготовить файл календаря.")
		}
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="ryden-meeting.ics"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(result); err != nil {
		s.logger.WarnContext(r.Context(), "calendar response write failed", "error", err)
	}
}

func (s *Server) handleMeetingPhotoGet(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	photo, err := s.media.GetMeetingPhoto(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	writePhoto(w, r, photo)
}

func (s *Server) handleMeetingPhotoPut(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	version, ok := parsePhotoVersion(w, r)
	if !ok {
		return
	}
	content, ok := readPhotoBody(w, r)
	if !ok {
		return
	}
	result, err := s.media.PutMeetingPhoto(
		r.Context(), mustUserID(r.Context()), meetingID, version,
		r.Header.Get("Content-Type"), content,
	)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	if result.Changed {
		s.metrics.PhotoChanged("meeting")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingPhotoDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	version, ok := parsePhotoVersion(w, r)
	if !ok {
		return
	}
	result, err := s.media.DeleteMeetingPhoto(
		r.Context(), mustUserID(r.Context()), meetingID, version,
	)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	if result.Changed {
		s.metrics.PhotoChanged("meeting")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePlanOptionPhotoGet(w http.ResponseWriter, r *http.Request) {
	meetingID, optionID, ok := parseMeetingOptionIDs(w, r)
	if !ok {
		return
	}
	photo, err := s.media.GetPlanOptionPhoto(
		r.Context(), mustUserID(r.Context()), meetingID, optionID,
	)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	writePhoto(w, r, photo)
}

func (s *Server) handlePlanOptionPhotoPut(w http.ResponseWriter, r *http.Request) {
	meetingID, optionID, ok := parseMeetingOptionIDs(w, r)
	if !ok {
		return
	}
	version, ok := parsePhotoVersion(w, r)
	if !ok {
		return
	}
	content, ok := readPhotoBody(w, r)
	if !ok {
		return
	}
	result, err := s.media.PutPlanOptionPhoto(
		r.Context(), mustUserID(r.Context()), meetingID, optionID, version,
		r.Header.Get("Content-Type"), content,
	)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	if result.Changed {
		s.metrics.PhotoChanged("plan_option")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePlanOptionPhotoDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, optionID, ok := parseMeetingOptionIDs(w, r)
	if !ok {
		return
	}
	version, ok := parsePhotoVersion(w, r)
	if !ok {
		return
	}
	result, err := s.media.DeletePlanOptionPhoto(
		r.Context(), mustUserID(r.Context()), meetingID, optionID, version,
	)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	if result.Changed {
		s.metrics.PhotoChanged("plan_option")
	}
	writeJSON(w, http.StatusOK, result)
}

func parseMeetingOptionIDs(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, uuid.UUID, bool) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	optionID, ok := parsePathUUID(w, r, "optionID", "option_not_found", "Вариант плана не найден.")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return meetingID, optionID, true
}

func parsePhotoVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || value < 1 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Передайте актуальную версию встречи.")
		return 0, false
	}
	return value, true
}

func readPhotoBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	return readPhotoBodyLimit(w, r, media.MaxPhotoBytes, "Фото должно быть не больше 3 МБ.")
}

func readPhotoBodyLimit(w http.ResponseWriter, r *http.Request, limit int64, message string) ([]byte, bool) {
	if r.ContentLength > limit {
		writeProblem(w, http.StatusRequestEntityTooLarge, "photo_too_large", message)
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "photo_too_large", message)
			return nil, false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_photo", "Не удалось прочитать фото.")
		return nil, false
	}
	return content, true
}

func writePhoto(w http.ResponseWriter, r *http.Request, photo media.Photo) {
	etag := `"` + hex.EncodeToString(photo.ContentHash) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Vary", "Authorization")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", photo.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(photo.Content)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(photo.Content); err != nil {
		// The request log still records the failed/short response without exposing image data.
		return
	}
}

func (s *Server) handleMeetingUpdate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input meeting.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, err := s.meetings.Update(r.Context(), mustUserID(r.Context()), meetingID, input)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	s.metrics.MeetingUpdated()
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingEvents(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	result, err := s.meetings.Get(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if s.live == nil {
		writeProblem(w, http.StatusServiceUnavailable, "live_unavailable", "Живые обновления временно недоступны.")
		return
	}
	subscription, err := s.live.Subscribe(meetingID, result.Version)
	if err != nil {
		if errors.Is(err, live.ErrMeetingLimit) || errors.Is(err, live.ErrSubscriberLimit) {
			w.Header().Set("Retry-After", "5")
			writeProblem(w, http.StatusServiceUnavailable, "live_capacity_reached", "Живые обновления временно перегружены.")
			return
		}
		s.logger.ErrorContext(r.Context(), "live subscription failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось подключить живые обновления.")
		return
	}
	defer subscription.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		s.logger.WarnContext(r.Context(), "disable live response deadline", "error", err)
	}
	if err := writeMeetingEvent(w, "ready", live.Event{Version: result.Version}); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-subscription.Events:
			if err := writeMeetingEvent(w, "meeting.updated", event); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func writeMeetingEvent(w io.Writer, eventName string, event live.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode meeting event: %w", err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, payload); err != nil {
		return fmt.Errorf("write meeting event: %w", err)
	}
	return nil
}

func (s *Server) handlePlanOptionCreate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input meeting.AddPlanOptionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.meetings.AddPlanOption(
		r.Context(), mustUserID(r.Context()), meetingID,
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handlePlanOptionDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	optionID, ok := parsePathUUID(w, r, "optionID", "option_not_found", "Вариант не найден.")
	if !ok {
		return
	}
	if err := s.meetings.DeletePlanOption(r.Context(), mustUserID(r.Context()), meetingID, optionID); err != nil {
		s.writeMeetingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePlanOptionUpdate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	optionID, ok := parsePathUUID(w, r, "optionID", "option_not_found", "Вариант не найден.")
	if !ok {
		return
	}
	var input meeting.UpdatePlanOptionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, err := s.meetings.UpdatePlanOption(
		r.Context(), mustUserID(r.Context()), meetingID, optionID, input,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	s.metrics.SetupOptionUpdated("plan")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTimeOptionCreate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input meeting.AddTimeOptionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.meetings.AddTimeOption(
		r.Context(), mustUserID(r.Context()), meetingID,
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleTimeOptionDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	optionID, ok := parsePathUUID(w, r, "optionID", "option_not_found", "Время не найдено.")
	if !ok {
		return
	}
	if err := s.meetings.DeleteTimeOption(r.Context(), mustUserID(r.Context()), meetingID, optionID); err != nil {
		s.writeMeetingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTimeOptionUpdate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	optionID, ok := parsePathUUID(w, r, "optionID", "option_not_found", "Время не найдено.")
	if !ok {
		return
	}
	var input meeting.UpdateTimeOptionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, err := s.meetings.UpdateTimeOption(
		r.Context(), mustUserID(r.Context()), meetingID, optionID, input,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	s.metrics.SetupOptionUpdated("time")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleInvitationCreate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input meeting.CreateInvitationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.meetings.CreateInvitation(
		r.Context(), mustUserID(r.Context()), meetingID,
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, result)
		return
	}
	s.metrics.InvitationCreated()
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleInvitationRevoke(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	if _, err := s.meetings.RevokeInvitation(r.Context(), mustUserID(r.Context()), meetingID); err != nil {
		s.writeMeetingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInvitationJoin(w http.ResponseWriter, r *http.Request) {
	var input meeting.JoinInvitationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, joined, err := s.meetings.JoinInvitation(r.Context(), mustUserID(r.Context()), input)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if joined {
		s.metrics.ParticipantJoined()
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePollList(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	result, err := s.polls.List(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		s.writePollError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (s *Server) handlePollCreate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input poll.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.polls.Create(
		r.Context(), mustUserID(r.Context()), meetingID,
		r.Header.Get("Idempotency-Key"), input,
	)
	if err != nil {
		s.writePollError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, result)
		return
	}
	s.metrics.PollCreated()
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handlePollHistory(w http.ResponseWriter, r *http.Request) {
	pollID, ok := parsePathUUID(w, r, "pollID", "poll_not_found", "Опрос не найден.")
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Проверьте параметры страницы.")
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Проверьте параметры страницы.")
		return
	}
	result, err := s.polls.History(
		r.Context(), mustUserID(r.Context()), pollID, limit, offset,
	)
	if err != nil {
		s.writePollError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePollDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	pollID, ok := parsePathUUID(w, r, "pollID", "poll_not_found", "Опрос не найден.")
	if !ok {
		return
	}
	if err := s.polls.Delete(r.Context(), mustUserID(r.Context()), meetingID, pollID); err != nil {
		s.writePollError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePollVote(w http.ResponseWriter, r *http.Request) {
	pollID, ok := parsePathUUID(w, r, "pollID", "poll_not_found", "Опрос не найден.")
	if !ok {
		return
	}
	var input poll.VoteInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.polls.Vote(r.Context(), mustUserID(r.Context()), pollID, input)
	if err != nil {
		s.writePollError(w, err)
		return
	}
	if changed {
		s.metrics.PollVoteSubmitted()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePollClose(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	pollID, ok := parsePathUUID(w, r, "pollID", "poll_not_found", "Опрос не найден.")
	if !ok {
		return
	}
	var input poll.CloseInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.polls.Close(
		r.Context(), mustUserID(r.Context()), meetingID, pollID, input,
	)
	if err != nil {
		s.writePollError(w, err)
		return
	}
	if changed {
		s.metrics.PollClosed(input.SelectedOptionID != nil)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAvailabilityList(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	result, err := s.availability.List(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		s.writeAvailabilityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAvailabilityRespond(w http.ResponseWriter, r *http.Request) {
	timeOptionID, ok := parsePathUUID(w, r, "timeOptionID", "time_option_not_found", "Вариант времени не найден.")
	if !ok {
		return
	}
	var input availability.RespondInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.availability.Respond(
		r.Context(), mustUserID(r.Context()), timeOptionID, input,
	)
	if err != nil {
		s.writeAvailabilityError(w, err)
		return
	}
	if changed {
		s.metrics.AvailabilityResponseSubmitted()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAttendanceGet(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		s.writeAttendanceError(w, err)
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		s.writeAttendanceError(w, err)
		return
	}
	result, err := s.attendance.Get(
		r.Context(), mustUserID(r.Context()), meetingID, limit, offset,
	)
	if err != nil {
		s.writeAttendanceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAttendanceRespond(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input attendance.RespondInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.attendance.Respond(
		r.Context(), mustUserID(r.Context()), meetingID, input,
	)
	if err != nil {
		s.writeAttendanceError(w, err)
		return
	}
	if changed {
		s.metrics.AttendanceResponseChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNoteList(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		s.writeNoteError(w, err)
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		s.writeNoteError(w, err)
		return
	}
	result, err := s.notes.List(r.Context(), mustUserID(r.Context()), meetingID, limit, offset)
	if err != nil {
		s.writeNoteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleNoteUpsert(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input note.UpsertInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.notes.Upsert(r.Context(), mustUserID(r.Context()), meetingID, input)
	if err != nil {
		s.writeNoteError(w, err)
		return
	}
	if changed {
		s.metrics.MeetingNoteChanged("upsert")
	}
	writeJSON(w, http.StatusOK, map[string]bool{"changed": changed})
}

func (s *Server) handleNoteDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	changed, err := s.notes.Delete(r.Context(), mustUserID(r.Context()), meetingID)
	if err != nil {
		s.writeNoteError(w, err)
		return
	}
	if changed {
		s.metrics.MeetingNoteChanged("delete")
	}
	writeJSON(w, http.StatusOK, map[string]bool{"changed": changed})
}

func (s *Server) handlePlanVoteList(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Проверьте параметры страницы.")
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Проверьте параметры страницы.")
		return
	}
	result, err := s.decisions.List(
		r.Context(), mustUserID(r.Context()), meetingID, limit, offset,
	)
	if err != nil {
		s.writeDecisionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePlanVote(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input decision.VoteInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.decisions.Vote(
		r.Context(), mustUserID(r.Context()), meetingID, input,
	)
	if err != nil {
		s.writeDecisionError(w, err)
		return
	}
	if changed {
		s.metrics.PlanVoteChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDecisionFinalize(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input decision.FinalizeInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.decisions.Finalize(
		r.Context(), mustUserID(r.Context()), meetingID, input,
	)
	if err != nil {
		s.writeDecisionError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	} else {
		s.metrics.DecisionFinalized()
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingComplete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(
		w, r, "meetingID", "meeting_not_found", "Встреча не найдена.",
	)
	if !ok {
		return
	}
	result, replayed, err := s.meetings.Complete(
		r.Context(), mustUserID(r.Context()), meetingID,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	} else {
		s.metrics.MeetingCompleted()
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingCancel(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(
		w, r, "meetingID", "meeting_not_found", "Встреча не найдена.",
	)
	if !ok {
		return
	}
	result, replayed, err := s.meetings.Cancel(
		r.Context(), mustUserID(r.Context()), meetingID,
	)
	if err != nil {
		s.writeMeetingError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	} else {
		s.metrics.MeetingCancelled()
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRequirementList(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Проверьте параметры страницы.")
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", "Проверьте параметры страницы.")
		return
	}
	result, err := s.preparation.List(
		r.Context(), mustUserID(r.Context()), meetingID, limit, offset,
	)
	if err != nil {
		s.writePreparationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRequirementCreate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var input preparation.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, replayed, err := s.preparation.Create(
		r.Context(),
		mustUserID(r.Context()),
		meetingID,
		r.Header.Get("Idempotency-Key"),
		input,
	)
	if err != nil {
		s.writePreparationError(w, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
		w.Header().Set("Idempotent-Replayed", "true")
	} else {
		s.metrics.RequirementCreated()
	}
	writeJSON(w, status, result)
}

func (s *Server) handleRequirementClaim(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	requirementID, ok := parsePathUUID(
		w, r, "requirementID", "requirement_not_found", "Позиция подготовки не найдена.",
	)
	if !ok {
		return
	}
	var input preparation.ClaimInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.preparation.SetClaim(
		r.Context(), mustUserID(r.Context()), meetingID, requirementID, input,
	)
	if err != nil {
		s.writePreparationError(w, err)
		return
	}
	if changed {
		s.metrics.RequirementClaimChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRequirementUpdate(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	requirementID, ok := parsePathUUID(
		w, r, "requirementID", "requirement_not_found", "Позиция подготовки не найдена.",
	)
	if !ok {
		return
	}
	var input preparation.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.preparation.Update(
		r.Context(), mustUserID(r.Context()), meetingID, requirementID, input,
	)
	if err != nil {
		s.writePreparationError(w, err)
		return
	}
	if changed {
		s.metrics.RequirementUpdated()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRequirementDelete(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	requirementID, ok := parsePathUUID(
		w, r, "requirementID", "requirement_not_found", "Позиция подготовки не найдена.",
	)
	if !ok {
		return
	}
	changed, err := s.preparation.Delete(
		r.Context(), mustUserID(r.Context()), meetingID, requirementID,
	)
	if err != nil {
		s.writePreparationError(w, err)
		return
	}
	if changed {
		s.metrics.RequirementDeleted()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRequirementStatus(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	requirementID, ok := parsePathUUID(
		w, r, "requirementID", "requirement_not_found", "Позиция подготовки не найдена.",
	)
	if !ok {
		return
	}
	var input preparation.StatusInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	changed, err := s.preparation.SetStatus(
		r.Context(), mustUserID(r.Context()), meetingID, requirementID, input,
	)
	if err != nil {
		s.writePreparationError(w, err)
		return
	}
	if changed {
		s.metrics.RequirementStatusChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "Необходимо войти.")
			return
		}
		userID, expiresAt, err := s.tokens.ParseAccessTokenWithExpiry(
			strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")),
		)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "Сессия истекла. Войдите снова.")
			return
		}
		authCtx, cancel := context.WithDeadline(r.Context(), expiresAt)
		defer cancel()
		authCtx = context.WithValue(authCtx, userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(authCtx))
	})
}

func (s *Server) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, auth.ErrEmailTaken):
		writeProblem(w, http.StatusConflict, "email_taken", "Аккаунт с такой почтой уже существует.")
	case errors.Is(err, auth.ErrNicknameTaken):
		writeProblem(w, http.StatusConflict, "nickname_taken", "Этот никнейм уже занят.")
	case errors.Is(err, auth.ErrInvalidLogin):
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Неверная почта или пароль.")
	case errors.Is(err, auth.ErrUnauthorized), errors.Is(err, auth.ErrSessionNotActive):
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "Сессия завершена. Войдите снова.")
	default:
		s.logger.Error("authentication request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}

func (s *Server) writeMeetingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, meeting.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, meeting.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "meeting_not_found", "Встреча не найдена.")
	case errors.Is(err, meeting.ErrOptionNotFound):
		writeProblem(w, http.StatusNotFound, "option_not_found", "Вариант встречи не найден.")
	case errors.Is(err, meeting.ErrIdempotencyConflict):
		writeProblem(w, http.StatusConflict, "idempotency_conflict", "Этот ключ повтора уже использован для другого запроса.")
	case errors.Is(err, meeting.ErrVersionConflict):
		writeProblem(w, http.StatusConflict, "version_conflict", "Встреча уже изменилась. Обновите данные и повторите.")
	case errors.Is(err, meeting.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "meeting_not_editable", "Детали этой встречи сейчас нельзя изменить.")
	case errors.Is(err, meeting.ErrSetupIncomplete):
		writeProblem(w, http.StatusConflict, "setup_incomplete", "Добавьте минимум два варианта плана и два варианта времени.")
	case errors.Is(err, meeting.ErrFixedSetupInvalid):
		writeProblem(w, http.StatusConflict, "fixed_setup_invalid", "Для готового плана оставьте ровно один план и один совместимый вариант времени.")
	case errors.Is(err, meeting.ErrOptionLimit):
		writeProblem(w, http.StatusConflict, "option_limit", "Для встречи можно добавить не более 20 вариантов этого типа.")
	case errors.Is(err, meeting.ErrDuplicateOption):
		writeProblem(w, http.StatusConflict, "duplicate_option", "Такой вариант времени уже добавлен.")
	case errors.Is(err, meeting.ErrNotCompletable):
		writeProblem(w, http.StatusConflict, "meeting_not_completable", "Завершить можно только встречу с закреплённым решением.")
	case errors.Is(err, meeting.ErrPreparationIncomplete):
		writeProblem(w, http.StatusConflict, "preparation_incomplete", "Сначала завершите все позиции подготовки.")
	case errors.Is(err, meeting.ErrNotCancellable):
		writeProblem(w, http.StatusConflict, "meeting_not_cancellable", "Завершённую встречу нельзя отменить.")
	case errors.Is(err, meeting.ErrInvitationInvalid):
		writeProblem(w, http.StatusGone, "invitation_invalid", "Ссылка приглашения недействительна или истекла.")
	default:
		s.logger.Error("meeting request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}

func (s *Server) writeMediaError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, media.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_photo", err.Error())
	case errors.Is(err, media.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "photo_not_found", "Фото не найдено или недоступно.")
	case errors.Is(err, media.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "photo_not_editable", "Фото этой встречи сейчас нельзя изменить.")
	case errors.Is(err, media.ErrVersionConflict):
		writeProblem(w, http.StatusConflict, "version_conflict", "Встреча уже изменилась. Обновите данные и повторите.")
	default:
		s.logger.Error("meeting photo request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие с фото.")
	}
}

func (s *Server) writePollError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, poll.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, poll.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "poll_not_found", "Опрос не найден.")
	case errors.Is(err, poll.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "poll_not_editable", "Сейчас это действие с опросом недоступно.")
	case errors.Is(err, poll.ErrLimit):
		writeProblem(w, http.StatusConflict, "poll_limit", "Для встречи можно создать не более 10 опросов.")
	case errors.Is(err, poll.ErrClosed):
		writeProblem(w, http.StatusConflict, "poll_closed", "Опрос уже закрыт.")
	case errors.Is(err, poll.ErrDeadline):
		writeProblem(w, http.StatusConflict, "poll_deadline_passed", "Срок ответа на опрос истёк.")
	case errors.Is(err, poll.ErrRevoteDisabled):
		writeProblem(w, http.StatusConflict, "poll_revote_disabled", "В этом опросе нельзя изменить уже отправленный ответ.")
	case errors.Is(err, poll.ErrConflict):
		writeProblem(w, http.StatusConflict, "poll_conflict", "Опрос уже закрыт с другим решением.")
	case errors.Is(err, poll.ErrIdempotencyConflict):
		writeProblem(w, http.StatusConflict, "idempotency_conflict", "Этот ключ повтора уже использован для другого запроса.")
	default:
		s.logger.Error("poll request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}

func (s *Server) writeAvailabilityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, availability.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, availability.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "time_option_not_found", "Вариант времени не найден.")
	case errors.Is(err, availability.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "availability_not_editable", "Доступность можно менять только во время сбора ответов.")
	default:
		s.logger.Error("availability request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}

func (s *Server) writeAttendanceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attendance.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, attendance.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "meeting_not_found", "Встреча не найдена.")
	case errors.Is(err, attendance.ErrNotAvailable):
		writeProblem(w, http.StatusConflict, "attendance_not_available", "Для этой встречи отдельный сбор участия не используется.")
	case errors.Is(err, attendance.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "attendance_not_editable", "Сейчас ответ об участии изменить нельзя.")
	default:
		s.logger.Error("attendance request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить участие.")
	}
}

func (s *Server) writeNoteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, note.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, note.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "meeting_not_found", "Встреча не найдена.")
	case errors.Is(err, note.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "notes_not_editable", "Заметки этой встречи доступны только для чтения.")
	default:
		s.logger.Error("meeting note request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось обновить заметку.")
	}
}

func (s *Server) writeDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, decision.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, decision.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "meeting_not_found", "Встреча не найдена.")
	case errors.Is(err, decision.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "decision_not_editable", "Сбор голосов за план уже завершён.")
	case errors.Is(err, decision.ErrIncompatible):
		writeProblem(w, http.StatusConflict, "incompatible_decision", "Выбранное время не подходит выбранному плану.")
	case errors.Is(err, decision.ErrConflict):
		writeProblem(w, http.StatusConflict, "decision_conflict", "Для встречи уже закреплено другое решение.")
	default:
		s.logger.Error("meeting decision request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}

func (s *Server) writePreparationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, preparation.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, preparation.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "requirement_not_found", "Позиция подготовки не найдена.")
	case errors.Is(err, preparation.ErrNotEditable):
		writeProblem(w, http.StatusConflict, "preparation_not_editable", "Подготовку сейчас нельзя изменить.")
	case errors.Is(err, preparation.ErrLimit):
		writeProblem(w, http.StatusConflict, "requirement_limit", "Для встречи можно добавить не более 50 позиций.")
	case errors.Is(err, preparation.ErrDuplicate):
		writeProblem(w, http.StatusConflict, "duplicate_requirement", "Такая позиция подготовки уже есть.")
	case errors.Is(err, preparation.ErrQuantityExceeded):
		writeProblem(w, http.StatusConflict, "requirement_quantity_exceeded", "Общее количество превышает необходимое.")
	case errors.Is(err, preparation.ErrNotFullyClaimed):
		writeProblem(w, http.StatusConflict, "requirement_not_fully_claimed", "Сначала распределите всё необходимое количество.")
	case errors.Is(err, preparation.ErrHasClaims):
		writeProblem(w, http.StatusConflict, "requirement_has_claims", "Сначала участники должны снять свои доли.")
	case errors.Is(err, preparation.ErrIdempotencyConflict):
		writeProblem(w, http.StatusConflict, "idempotency_conflict", "Этот ключ повтора уже использован для другой позиции.")
	default:
		s.logger.Error("preparation request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, session auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    session.RefreshToken,
		Path:     "/api/v1/auth",
		Expires:  session.RefreshUntil,
		MaxAge:   int(time.Until(session.RefreshUntil).Seconds()),
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: "", Path: "/api/v1/auth",
		MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.ErrorContext(r.Context(), "panic recovered", "panic", recovered)
				writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
	})
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.InfoContext(r.Context(), "http request",
			"request_id", requestID(r.Context()),
			"method", r.Method,
			"route_group", observabilityRouteGroup(r.URL.Path),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) timeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == s.allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, meeting.ErrInvalidInput
	}
	return parsed, nil
}

func parsePathUUID(
	w http.ResponseWriter,
	r *http.Request,
	name, code, message string,
) (uuid.UUID, bool) {
	value, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeProblem(w, http.StatusNotFound, code, message)
		return uuid.Nil, false
	}
	return value, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func mustUserID(ctx context.Context) uuid.UUID {
	userID, _ := ctx.Value(userIDKey).(uuid.UUID)
	return userID
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func newRequestID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(value)
}

func observabilityRouteGroup(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/v1/auth/"):
		return "auth"
	case strings.HasPrefix(path, "/api/v1/meetings"):
		return "meetings"
	case strings.HasPrefix(path, "/api/v1/time-options"):
		return "meetings"
	case strings.HasPrefix(path, "/api/v1/me/meeting-invitations"):
		return "meeting_invitations"
	case path == "/api/v1/me":
		return "profile"
	case path == "/api/v1/me/avatar", strings.HasSuffix(path, "/avatar") && strings.HasPrefix(path, "/api/v1/users/"):
		return "profile"
	case strings.HasPrefix(path, "/api/v1/users/search"), strings.HasPrefix(path, "/api/v1/friends"), strings.HasPrefix(path, "/api/v1/friend-requests"):
		return "friends"
	case path == "/metrics":
		return "metrics"
	case strings.HasSuffix(path, "z"):
		return "health"
	default:
		return "other"
	}
}
