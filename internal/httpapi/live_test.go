package httpapi

import (
	"bytes"
	"testing"

	"github.com/ryden-app/ryden/internal/live"
)

func TestWriteMeetingEvent(t *testing.T) {
	var output bytes.Buffer
	if err := writeMeetingEvent(&output, "meeting.updated", live.Event{Version: 7}); err != nil {
		t.Fatalf("write event: %v", err)
	}
	const expected = "event: meeting.updated\ndata: {\"version\":7}\n\n"
	if output.String() != expected {
		t.Fatalf("event = %q, want %q", output.String(), expected)
	}
}
