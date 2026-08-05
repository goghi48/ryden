package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/ryden-app/ryden/internal/friendship"
)

func (s *Server) handleUserSearch(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	items, err := s.friends.Search(r.Context(), mustUserID(r.Context()), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFriendsOverview(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	overview, err := s.friends.Overview(r.Context(), mustUserID(r.Context()), limit, offset)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

type sendFriendRequest struct {
	UserID uuid.UUID `json:"user_id"`
}

func (s *Server) handleFriendRequestSend(w http.ResponseWriter, r *http.Request) {
	var request sendFriendRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Проверьте формат данных.")
		return
	}
	result, err := s.friends.Send(r.Context(), mustUserID(r.Context()), request.UserID)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	if result.Changed {
		s.metrics.FriendshipChanged("send")
		writeJSON(w, http.StatusCreated, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFriendRequestAccept(w http.ResponseWriter, r *http.Request) {
	requestID, ok := parsePathUUID(w, r, "requestID", "friend_request_not_found", "Заявка не найдена.")
	if !ok {
		return
	}
	result, err := s.friends.Accept(r.Context(), mustUserID(r.Context()), requestID)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	if result.Changed {
		s.metrics.FriendshipChanged("accept")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFriendRequestDelete(w http.ResponseWriter, r *http.Request) {
	requestID, ok := parsePathUUID(w, r, "requestID", "friend_request_not_found", "Заявка не найдена.")
	if !ok {
		return
	}
	result, err := s.friends.DeleteRequest(r.Context(), mustUserID(r.Context()), requestID)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	if result.Changed {
		s.metrics.FriendshipChanged("request_remove")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFriendRemove(w http.ResponseWriter, r *http.Request) {
	friendID, ok := parsePathUUID(w, r, "friendID", "friend_not_found", "Друг не найден.")
	if !ok {
		return
	}
	result, err := s.friends.RemoveFriend(r.Context(), mustUserID(r.Context()), friendID)
	if err != nil {
		s.writeFriendshipError(w, err)
		return
	}
	if result.Changed {
		s.metrics.FriendshipChanged("friend_remove")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeFriendshipError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, friendship.ErrInvalidInput):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_input", err.Error())
	case errors.Is(err, friendship.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "friendship_not_found", "Пользователь или заявка не найдены.")
	default:
		s.logger.Error("friendship request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить действие.")
	}
}
