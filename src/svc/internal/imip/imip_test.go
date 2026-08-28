package imip

import (
	"strings"
	"testing"
)

func TestBuildContainsCalendar(t *testing.T) {
	raw := Build("Ada <ada@example.com>", "bob@example.com", "Invitation: Standup", "Please join.", "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nEND:VCALENDAR\r\n", "")
	s := string(raw)
	if !strings.Contains(s, "method=REQUEST") || !strings.Contains(s, "text/calendar") {
		t.Fatalf("missing calendar part:\n%s", s)
	}
	if !strings.Contains(s, "To: bob@example.com") || !strings.Contains(s, "METHOD:REQUEST") {
		t.Fatalf("missing headers:\n%s", s)
	}
}
