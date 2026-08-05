package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ryden-app/ryden/internal/meeting"
)

type meetingReaderStub struct {
	detail    meeting.Detail
	err       error
	userID    uuid.UUID
	meetingID uuid.UUID
}

func (s *meetingReaderStub) Get(
	_ context.Context,
	userID, meetingID uuid.UUID,
) (meeting.Detail, error) {
	s.userID = userID
	s.meetingID = meetingID
	return s.detail, s.err
}

func TestExportBuildsStableCompatibleCalendarEvent(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	meetingID := uuid.New()
	planID := uuid.New()
	timeID := uuid.New()
	locationName := "Дом Анны, второй этаж"
	locationURL := "https://example.test/place?id=1,2"
	reader := &meetingReaderStub{detail: meeting.Detail{
		Meeting: meeting.Meeting{
			ID: meetingID, Title: "Ужин; настольные игры",
			Description:  "Берём еду,\r\nигры и хорошее настроение.",
			LocationName: &locationName, LocationURL: &locationURL,
			Timezone: "Asia/Novosibirsk", State: "scheduled",
			SelectedPlanOptionID: &planID, SelectedTimeOptionID: &timeID,
			Version: 7, UpdatedAt: time.Date(2026, 8, 1, 10, 20, 30, 0, time.FixedZone("NOVT", 7*60*60)),
		},
		PlanOptions: []meeting.PlanOption{{
			ID: planID, Title: "Очень длинный русский план с ужином и настольными играми для всей компании",
		}},
		TimeOptions: []meeting.TimeOption{{
			ID:       timeID,
			StartsAt: time.Date(2026, 8, 10, 19, 0, 0, 0, time.FixedZone("NOVT", 7*60*60)),
			EndsAt:   timePointer(time.Date(2026, 8, 10, 22, 0, 0, 0, time.FixedZone("NOVT", 7*60*60))),
		}},
	}}
	service := NewService(reader)
	service.now = func() time.Time {
		return time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC)
	}

	data, err := service.Export(context.Background(), userID, meetingID)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if reader.userID != userID || reader.meetingID != meetingID {
		t.Fatalf("Get() IDs = (%s, %s), want (%s, %s)", reader.userID, reader.meetingID, userID, meetingID)
	}
	result := string(data)
	expectedParts := []string{
		"BEGIN:VCALENDAR\r\n",
		"UID:" + meetingID.String() + "@ryden.app\r\n",
		"DTSTAMP:20260801T010203Z\r\n",
		"DTSTART:20260810T120000Z\r\n",
		"DTEND:20260810T150000Z\r\n",
		"LAST-MODIFIED:20260801T032030Z\r\n",
		"SEQUENCE:7\r\n",
		"STATUS:CONFIRMED\r\n",
		"SUMMARY:Ужин\\; настольные игры\r\n",
		"LOCATION:Дом Анны\\, второй этаж\r\n",
		"X-RYDEN-TIMEZONE:Asia/Novosibirsk\r\n",
		"END:VCALENDAR\r\n",
	}
	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("calendar does not contain %q:\n%s", part, result)
		}
	}
	unfolded := strings.ReplaceAll(result, "\r\n ", "")
	if !strings.Contains(unfolded, "Берём еду\\,\\nигры") ||
		!strings.Contains(unfolded, "Ссылка на место: https://example.test/place?id=1\\,2") {
		t.Fatalf("escaped description is missing:\n%s", unfolded)
	}
	for _, line := range strings.Split(strings.TrimSuffix(result, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("content line has %d octets, want <= 75: %q", len(line), line)
		}
		if !utf8.ValidString(line) {
			t.Errorf("content line splits a UTF-8 rune: %q", line)
		}
	}
}

func TestExportRejectsMeetingWithoutConfirmedDecision(t *testing.T) {
	t.Parallel()

	service := NewService(&meetingReaderStub{detail: meeting.Detail{
		Meeting: meeting.Meeting{State: "collecting"},
	}})
	_, err := service.Export(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("Export() error = %v, want ErrNotAvailable", err)
	}
}

func TestExportOmitsEndWhenDurationIsUnknown(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	timeID := uuid.New()
	service := NewService(&meetingReaderStub{detail: meeting.Detail{
		Meeting: meeting.Meeting{
			ID: planID, Title: "Встреча", State: "scheduled",
			SelectedPlanOptionID: &planID, SelectedTimeOptionID: &timeID,
		},
		PlanOptions: []meeting.PlanOption{{ID: planID, Title: "Встреча"}},
		TimeOptions: []meeting.TimeOption{{
			ID: timeID, StartsAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		}},
	}})

	data, err := service.Export(context.Background(), uuid.New(), planID)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	result := string(data)
	if !strings.Contains(result, "DTSTART:20260810T120000Z\r\n") {
		t.Fatalf("calendar does not contain DTSTART:\n%s", result)
	}
	if strings.Contains(result, "DTEND:") {
		t.Fatalf("calendar unexpectedly contains DTEND:\n%s", result)
	}
}

func TestExportRejectsBrokenConfirmedDecision(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	timeID := uuid.New()
	service := NewService(&meetingReaderStub{detail: meeting.Detail{
		Meeting: meeting.Meeting{
			State: "completed", SelectedPlanOptionID: &planID, SelectedTimeOptionID: &timeID,
		},
	}})
	_, err := service.Export(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("Export() error = %v, want ErrInvalidDecision", err)
	}
}

func TestExportRejectsTimeForAnotherPlan(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	otherPlanID := uuid.New()
	timeID := uuid.New()
	service := NewService(&meetingReaderStub{detail: meeting.Detail{
		Meeting: meeting.Meeting{
			State: "scheduled", SelectedPlanOptionID: &planID, SelectedTimeOptionID: &timeID,
		},
		PlanOptions: []meeting.PlanOption{{ID: planID, Title: "Ужин"}},
		TimeOptions: []meeting.TimeOption{{
			ID: timeID, PlanOptionID: &otherPlanID,
			StartsAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
			EndsAt:   timePointer(time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)),
		}},
	}})
	_, err := service.Export(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("Export() error = %v, want ErrInvalidDecision", err)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func TestExportPreservesMeetingAuthorizationError(t *testing.T) {
	t.Parallel()

	service := NewService(&meetingReaderStub{err: meeting.ErrNotFound})
	_, err := service.Export(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, meeting.ErrNotFound) {
		t.Fatalf("Export() error = %v, want meeting.ErrNotFound", err)
	}
}
