package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/ryden-app/ryden/internal/meetinginvite"
)

func (s *Server) handleMeetingInviteCandidates(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	limit, limitErr := queryInt(r, "limit", 50)
	offset, offsetErr := queryInt(r, "offset", 0)
	if limitErr != nil || offsetErr != nil {
		s.writeMeetingInviteError(w, meetinginvite.ErrInvalidInput)
		return
	}
	result, err := s.meetingInvites.Candidates(
		r.Context(), mustUserID(r.Context()), meetingID, limit, offset,
	)
	if err != nil {
		s.writeMeetingInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type sendMeetingInvitesRequest struct {
	UserIDs []uuid.UUID `json:"user_ids"`
}

func (s *Server) handleMeetingInviteSend(w http.ResponseWriter, r *http.Request) {
	meetingID, ok := parsePathUUID(w, r, "meetingID", "meeting_not_found", "Встреча не найдена.")
	if !ok {
		return
	}
	var request sendMeetingInvitesRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, err := s.meetingInvites.Send(
		r.Context(), mustUserID(r.Context()), meetingID, request.UserIDs,
	)
	if err != nil {
		s.writeMeetingInviteError(w, err)
		return
	}
	if result.ChangedCount > 0 {
		s.metrics.MeetingInviteChanged("send", result.ChangedCount)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingInviteIncoming(w http.ResponseWriter, r *http.Request) {
	limit, limitErr := queryInt(r, "limit", 50)
	offset, offsetErr := queryInt(r, "offset", 0)
	if limitErr != nil || offsetErr != nil {
		s.writeMeetingInviteError(w, meetinginvite.ErrInvalidInput)
		return
	}
	result, err := s.meetingInvites.Incoming(
		r.Context(), mustUserID(r.Context()), limit, offset,
	)
	if err != nil {
		s.writeMeetingInviteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingInviteAccept(w http.ResponseWriter, r *http.Request) {
	invitationID, ok := parsePathUUID(
		w, r, "invitationID", "meeting_invitation_not_found", "Приглашение не найдено.",
	)
	if !ok {
		return
	}
	result, err := s.meetingInvites.Accept(r.Context(), mustUserID(r.Context()), invitationID)
	if err != nil {
		s.writeMeetingInviteError(w, err)
		return
	}
	if result.Changed {
		s.metrics.MeetingInviteChanged("accept", 1)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeetingInviteDecline(w http.ResponseWriter, r *http.Request) {
	invitationID, ok := parsePathUUID(
		w, r, "invitationID", "meeting_invitation_not_found", "Приглашение не найдено.",
	)
	if !ok {
		return
	}
	result, err := s.meetingInvites.Decline(r.Context(), mustUserID(r.Context()), invitationID)
	if err != nil {
		s.writeMeetingInviteError(w, err)
		return
	}
	if result.Changed {
		s.metrics.MeetingInviteChanged("decline", 1)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) writeMeetingInviteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, meetinginvite.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, meetinginvite.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "meeting_invitation_not_found", "Приглашение или встреча не найдены.")
	case errors.Is(err, meetinginvite.ErrConflict):
		writeProblem(w, http.StatusConflict, "meeting_invitation_conflict", "На это приглашение уже нельзя ответить.")
	default:
		s.logger.Error("direct meeting invitation request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие с приглашением.")
	}
}
