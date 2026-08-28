package icsutil_test

import (
	"testing"
	"time"

	"github.com/devcoons/dcalcon/internal/icsutil"
)

func TestOverlapsTimeRangeRRULE(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:weekly@dcalcon\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := icsutil.ParseCalendar(ics)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if !icsutil.OverlapsTimeRange(cal, start, end) {
		t.Fatal("weekly event starting in January should match an August window")
	}
	missStart := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) // Tuesday; 5 Jan 2026 was Monday
	missEnd := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if icsutil.OverlapsTimeRange(cal, missStart, missEnd) {
		t.Fatal("should not match a Tuesday-only window")
	}
}

func TestSetOrganizerAttendees(t *testing.T) {
	raw := icsutil.NewEventICS("e1", "Lunch", "20260828T120000Z", "20260828T130000Z", "", "")
	out, err := icsutil.SetOrganizerAttendees(raw, "alice@example.com", "Alice", []icsutil.AttendeeInfo{
		{Value: "bob@example.com", CN: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if icsutil.OrganizerValue(out) == "" {
		t.Fatal("missing organizer")
	}
	atts := icsutil.Attendees(out)
	if len(atts) != 1 || icsutil.AddrOf(atts[0].Value) != "bob@example.com" {
		t.Fatalf("attendees %+v", atts)
	}
}

func TestMergeOrganizerAttendees(t *testing.T) {
	raw := icsutil.NewEventICS("e1", "Lunch", "20260828T120000Z", "20260828T130000Z", "", "")
	out, err := icsutil.SetOrganizerAttendees(raw, "alice@example.com", "Alice", []icsutil.AttendeeInfo{
		{Value: "bob@example.com", CN: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err = icsutil.MergeOrganizerAttendees(out, "alice@example.com", "Alice", []icsutil.AttendeeInfo{
		{Value: "bob@example.com", CN: "Bob"},
		{Value: "cara@example.com", CN: "Cara"},
	})
	if err != nil {
		t.Fatal(err)
	}
	atts := icsutil.Attendees(out)
	if len(atts) != 2 {
		t.Fatalf("want 2 attendees, got %+v", atts)
	}
}
