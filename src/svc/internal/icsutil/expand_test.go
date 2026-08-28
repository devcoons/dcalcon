package icsutil_test

import (
	"strings"
	"testing"
	"time"

	"github.com/devcoons/dcalcon/internal/icsutil"
)

func TestExpandWeeklyInAugust(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:weekly@dcalcon\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := icsutil.ParseCalendar(ics)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	exp := icsutil.ExpandCalendar(cal, start, end)
	raw, err := icsutil.EncodeCalendar(exp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "RECURRENCE-ID") {
		t.Fatalf("expected expanded instances, got %s", raw)
	}
	if strings.Count(raw, "BEGIN:VEVENT") < 3 {
		t.Fatalf("expected several August instances, got %s", raw)
	}
}

func TestFirstOccurrenceFindsNextWeekly(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:weekly@dcalcon\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	ds, _, ok := icsutil.FirstOccurrence(ics, start, end)
	if !ok || !strings.HasPrefix(ds, "202608") {
		t.Fatalf("next weekly in window %q ok=%v", ds, ok)
	}
}
