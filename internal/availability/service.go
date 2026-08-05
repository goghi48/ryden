package availability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPreferred   Status = "preferred"
	StatusAvailable   Status = "available"
	StatusIfNeeded    Status = "if_needed"
	StatusUnavailable Status = "unavailable"
	StatusUnanswered  Status = "unanswered"
)

const (
	preferredWeight   = 3
	availableWeight   = 2
	ifNeededWeight    = 1
	unavailableWeight = -4
)

var (
	ErrInvalidInput = errors.New("invalid availability input")
	ErrNotFound     = errors.New("availability option not found")
	ErrNotEditable  = errors.New("availability is not editable")
)

type Repository interface {
	Snapshot(ctx context.Context, userID, meetingID uuid.UUID) (Snapshot, error)
	SetStatus(ctx context.Context, userID, timeOptionID uuid.UUID, status Status) (bool, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

type PlanOption struct {
	ID    uuid.UUID
	Title string
}

type TimeOption struct {
	ID           uuid.UUID
	PlanOptionID *uuid.UUID
	StartsAt     time.Time
	EndsAt       *time.Time
	Position     int16
}

type Participant struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
}

type Vote struct {
	TimeOptionID uuid.UUID
	UserID       uuid.UUID
	Status       Status
}

type Snapshot struct {
	Plans        []PlanOption
	Times        []TimeOption
	Participants []Participant
	Votes        []Vote
}

type Weights struct {
	Preferred   int `json:"preferred"`
	Available   int `json:"available"`
	IfNeeded    int `json:"if_needed"`
	Unavailable int `json:"unavailable"`
	Unanswered  int `json:"unanswered"`
}

type Counts struct {
	Preferred   int `json:"preferred"`
	Available   int `json:"available"`
	IfNeeded    int `json:"if_needed"`
	Unavailable int `json:"unavailable"`
	Unanswered  int `json:"unanswered"`
}

type Response struct {
	UserID uuid.UUID `json:"user_id"`
	Status Status    `json:"status"`
}

type TimeResult struct {
	ID           uuid.UUID  `json:"id"`
	PlanOptionID *uuid.UUID `json:"plan_option_id"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	Position     int16      `json:"position"`
	MyStatus     Status     `json:"my_status"`
	Counts       Counts     `json:"counts"`
	Responses    []Response `json:"responses"`
	Score        int        `json:"score"`
}

type Recommendation struct {
	PlanOptionID uuid.UUID `json:"plan_option_id"`
	PlanTitle    string    `json:"plan_title"`
	TimeOptionID uuid.UUID `json:"time_option_id"`
	Score        int       `json:"score"`
	Provisional  bool      `json:"provisional"`
	Explanation  string    `json:"explanation"`
}

type View struct {
	Weights         Weights          `json:"weights"`
	Participants    []Participant    `json:"participants"`
	Items           []TimeResult     `json:"items"`
	Recommendations []Recommendation `json:"recommendations"`
}

type RespondInput struct {
	Status Status `json:"status"`
}

func (s *Service) List(ctx context.Context, userID, meetingID uuid.UUID) (View, error) {
	snapshot, err := s.repository.Snapshot(ctx, userID, meetingID)
	if err != nil {
		return View{}, err
	}
	return buildView(userID, snapshot), nil
}

func (s *Service) Respond(
	ctx context.Context,
	userID, timeOptionID uuid.UUID,
	input RespondInput,
) (bool, error) {
	if !validInputStatus(input.Status) {
		return false, fmt.Errorf("%w: unknown status", ErrInvalidInput)
	}
	return s.repository.SetStatus(ctx, userID, timeOptionID, input.Status)
}

func validInputStatus(status Status) bool {
	switch status {
	case StatusPreferred, StatusAvailable, StatusIfNeeded, StatusUnavailable, StatusUnanswered:
		return true
	default:
		return false
	}
}

func buildView(userID uuid.UUID, snapshot Snapshot) View {
	view := View{
		Weights: Weights{
			Preferred: preferredWeight, Available: availableWeight,
			IfNeeded: ifNeededWeight, Unavailable: unavailableWeight,
		},
		Participants:    snapshot.Participants,
		Items:           make([]TimeResult, len(snapshot.Times)),
		Recommendations: make([]Recommendation, 0, len(snapshot.Plans)),
	}
	byTime := make(map[uuid.UUID]*TimeResult, len(snapshot.Times))
	for index, option := range snapshot.Times {
		view.Items[index] = TimeResult{
			ID: option.ID, PlanOptionID: option.PlanOptionID,
			StartsAt: option.StartsAt, EndsAt: option.EndsAt, Position: option.Position,
			MyStatus:  StatusUnanswered,
			Counts:    Counts{Unanswered: len(snapshot.Participants)},
			Responses: make([]Response, 0),
		}
		byTime[option.ID] = &view.Items[index]
	}
	for _, vote := range snapshot.Votes {
		item := byTime[vote.TimeOptionID]
		if item == nil {
			continue
		}
		item.Responses = append(item.Responses, Response{UserID: vote.UserID, Status: vote.Status})
		item.Counts.Unanswered--
		switch vote.Status {
		case StatusPreferred:
			item.Counts.Preferred++
			item.Score += preferredWeight
		case StatusAvailable:
			item.Counts.Available++
			item.Score += availableWeight
		case StatusIfNeeded:
			item.Counts.IfNeeded++
			item.Score += ifNeededWeight
		case StatusUnavailable:
			item.Counts.Unavailable++
			item.Score += unavailableWeight
		}
		if vote.UserID == userID {
			item.MyStatus = vote.Status
		}
	}
	for _, plan := range snapshot.Plans {
		var best *TimeResult
		for index := range view.Items {
			item := &view.Items[index]
			if item.PlanOptionID != nil && *item.PlanOptionID != plan.ID {
				continue
			}
			if best == nil || ranksBefore(*item, *best) {
				best = item
			}
		}
		if best == nil {
			continue
		}
		view.Recommendations = append(view.Recommendations, Recommendation{
			PlanOptionID: plan.ID,
			PlanTitle:    plan.Title,
			TimeOptionID: best.ID,
			Score:        best.Score,
			Provisional:  best.Counts.Unanswered > 0,
			Explanation:  explain(*best, len(snapshot.Participants)),
		})
	}
	for index := range view.Items {
		sortResponses(view.Items[index].Responses)
	}
	return view
}

func ranksBefore(left, right TimeResult) bool {
	if left.Counts.Unavailable != right.Counts.Unavailable {
		return left.Counts.Unavailable < right.Counts.Unavailable
	}
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.Counts.Preferred != right.Counts.Preferred {
		return left.Counts.Preferred > right.Counts.Preferred
	}
	leftAnswered := totalAnswered(left.Counts)
	rightAnswered := totalAnswered(right.Counts)
	if leftAnswered != rightAnswered {
		return leftAnswered > rightAnswered
	}
	if !left.StartsAt.Equal(right.StartsAt) {
		return left.StartsAt.Before(right.StartsAt)
	}
	if left.Position != right.Position {
		return left.Position < right.Position
	}
	return left.ID.String() < right.ID.String()
}

func totalAnswered(counts Counts) int {
	return counts.Preferred + counts.Available + counts.IfNeeded + counts.Unavailable
}

func explain(item TimeResult, participantCount int) string {
	answered := totalAnswered(item.Counts)
	if answered == 0 {
		return "Пока нет ответов: выбран самый ранний совместимый вариант."
	}
	prefix := "Нет ответов «не могу»."
	if item.Counts.Unavailable > 0 {
		prefix = fmt.Sprintf("Ответов «не могу»: %d.", item.Counts.Unavailable)
	}
	return fmt.Sprintf(
		"%s Предпочитают: %d, могут: %d, при необходимости: %d. Ответили %d из %d, итоговый вес %d.",
		prefix,
		item.Counts.Preferred,
		item.Counts.Available,
		item.Counts.IfNeeded,
		answered,
		participantCount,
		item.Score,
	)
}

func sortResponses(responses []Response) {
	sort.Slice(responses, func(i, j int) bool {
		return responses[i].UserID.String() < responses[j].UserID.String()
	})
}
