package calendar

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ryden-app/ryden/internal/meeting"
)

var (
	ErrNotAvailable    = errors.New("calendar export is not available")
	ErrInvalidDecision = errors.New("confirmed meeting has an invalid decision")
)

type MeetingReader interface {
	Get(context.Context, uuid.UUID, uuid.UUID) (meeting.Detail, error)
}

type Service struct {
	meetings MeetingReader
	now      func() time.Time
}

func NewService(meetings MeetingReader) *Service {
	return &Service{
		meetings: meetings,
		now:      time.Now,
	}
}

func (s *Service) Export(ctx context.Context, userID, meetingID uuid.UUID) ([]byte, error) {
	detail, err := s.meetings.Get(ctx, userID, meetingID)
	if err != nil {
		return nil, err
	}
	if detail.State != "scheduled" && detail.State != "completed" {
		return nil, ErrNotAvailable
	}
	if detail.SelectedPlanOptionID == nil || detail.SelectedTimeOptionID == nil {
		return nil, ErrInvalidDecision
	}

	var selectedPlan *meeting.PlanOption
	for index := range detail.PlanOptions {
		if detail.PlanOptions[index].ID == *detail.SelectedPlanOptionID {
			selectedPlan = &detail.PlanOptions[index]
			break
		}
	}
	var selectedTime *meeting.TimeOption
	for index := range detail.TimeOptions {
		if detail.TimeOptions[index].ID == *detail.SelectedTimeOptionID {
			selectedTime = &detail.TimeOptions[index]
			break
		}
	}
	if selectedPlan == nil ||
		selectedTime == nil ||
		(selectedTime.PlanOptionID != nil && *selectedTime.PlanOptionID != selectedPlan.ID) ||
		(selectedTime.EndsAt != nil && !selectedTime.EndsAt.After(selectedTime.StartsAt)) {
		return nil, ErrInvalidDecision
	}

	descriptionParts := make([]string, 0, 4)
	if detail.Description != "" {
		descriptionParts = append(descriptionParts, detail.Description)
	}
	descriptionParts = append(descriptionParts, "План: "+selectedPlan.Title)
	if detail.LocationURL != nil {
		descriptionParts = append(descriptionParts, "Ссылка на место: "+*detail.LocationURL)
	}
	descriptionParts = append(descriptionParts, "Часовой пояс встречи: "+detail.Timezone)

	var result strings.Builder
	writeLine(&result, "BEGIN:VCALENDAR")
	writeLine(&result, "VERSION:2.0")
	writeLine(&result, "PRODID:-//Ryden//Meeting Calendar Export//RU")
	writeLine(&result, "CALSCALE:GREGORIAN")
	writeLine(&result, "METHOD:PUBLISH")
	writeLine(&result, "BEGIN:VEVENT")
	writeLine(&result, "UID:"+meetingID.String()+"@ryden.app")
	writeLine(&result, "DTSTAMP:"+formatUTC(s.now()))
	writeLine(&result, "DTSTART:"+formatUTC(selectedTime.StartsAt))
	if selectedTime.EndsAt != nil {
		writeLine(&result, "DTEND:"+formatUTC(*selectedTime.EndsAt))
	}
	writeLine(&result, "LAST-MODIFIED:"+formatUTC(detail.UpdatedAt))
	writeLine(&result, "SEQUENCE:"+strconv.FormatInt(detail.Version, 10))
	writeLine(&result, "STATUS:CONFIRMED")
	writeLine(&result, "SUMMARY:"+escapeText(detail.Title))
	writeLine(&result, "DESCRIPTION:"+escapeText(strings.Join(descriptionParts, "\n\n")))
	if detail.LocationName != nil {
		writeLine(&result, "LOCATION:"+escapeText(*detail.LocationName))
	}
	writeLine(&result, "X-RYDEN-TIMEZONE:"+escapeText(detail.Timezone))
	writeLine(&result, "END:VEVENT")
	writeLine(&result, "END:VCALENDAR")
	return []byte(result.String()), nil
}

func formatUTC(value time.Time) string {
	return value.UTC().Format("20060102T150405Z")
}

func escapeText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, ";", "\\;")
	return strings.ReplaceAll(value, ",", "\\,")
}

func writeLine(target *strings.Builder, line string) {
	target.WriteString(foldLine(line))
	target.WriteString("\r\n")
}

func foldLine(line string) string {
	if len(line) <= 75 {
		return line
	}
	var result strings.Builder
	lineBytes := 0
	for _, character := range line {
		size := utf8.RuneLen(character)
		if size < 0 {
			size = len(string(character))
		}
		if lineBytes+size > 75 {
			result.WriteString("\r\n ")
			lineBytes = 1
		}
		result.WriteRune(character)
		lineBytes += size
	}
	return result.String()
}
