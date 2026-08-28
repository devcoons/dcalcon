package icsutil

import (
	"strings"
	"testing"
)

func TestUpdateEventICSPreservesRecurrenceAndTZID(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:e1\r\nDTSTAMP:20260828T080000Z\r\nDTSTART;TZID=Europe/Athens:20260828T090000\r\nDTEND;TZID=Europe/Athens:20260828T100000\r\nSUMMARY:Standup\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE\r\nEXDATE;TZID=Europe/Athens:20260902T090000\r\nX-APPLE-STRUCTURED-LOCATION;VALUE=URI:geo:37.98,23.72\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	updated, err := UpdateEventICS(raw, "Standup notes", "20260828T090000", "20260828T100000", "", "")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := SetRRule(updated, "FREQ=WEEKLY")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(merged, "TZID=Europe/Athens") {
		t.Fatalf("TZID dropped:\n%s", merged)
	}
	if !strings.Contains(merged, "BYDAY=MO,WE") {
		t.Fatalf("BYDAY dropped:\n%s", merged)
	}
	if !strings.Contains(merged, "EXDATE") || !strings.Contains(merged, "20260902T090000") {
		t.Fatalf("EXDATE dropped:\n%s", merged)
	}
	if !strings.Contains(merged, "X-APPLE-STRUCTURED-LOCATION") {
		t.Fatalf("client X- prop dropped:\n%s", merged)
	}
	if !strings.Contains(merged, "SUMMARY:Standup notes") {
		t.Fatalf("summary not updated:\n%s", merged)
	}
	cleared, err := SetRRule(merged, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cleared, "RRULE") {
		t.Fatalf("RRULE should be gone:\n%s", cleared)
	}
	if !strings.Contains(cleared, "EXDATE") {
		t.Fatalf("clearing RRULE must keep EXDATE:\n%s", cleared)
	}
}

func TestSetDatePropCopiesTZIDWhenTimeChanges(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nBEGIN:VEVENT\r\nUID:e2\r\nDTSTART;TZID=Europe/Athens:20260828T090000\r\nDTEND;TZID=Europe/Athens:20260828T100000\r\nSUMMARY:Move\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	updated, err := UpdateEventICS(raw, "Move", "20260828T110000", "20260828T120000", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated, "DTSTART;TZID=Europe/Athens:20260828T110000") {
		t.Fatalf("moved DTSTART lost TZID:\n%s", updated)
	}
	if !strings.Contains(updated, "DTEND;TZID=Europe/Athens:20260828T120000") {
		t.Fatalf("moved DTEND lost TZID:\n%s", updated)
	}
}

func TestEventLocationAndAllDay(t *testing.T) {
	raw := NewEventICS("u1", "Offsite", "20260828", "20260829", "Bring laptop", "Athens")
	if !AllDayFromICS(raw) {
		t.Fatal("expected all-day")
	}
	if LocationFromICS(raw) != "Athens" {
		t.Fatalf("location %q", LocationFromICS(raw))
	}
	updated, err := UpdateEventICS(raw, "Offsite 2", "20260828T090000", "20260828T100000", "notes", "Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if AllDayFromICS(updated) {
		t.Fatal("timed event should not be all-day")
	}
	if LocationFromICS(updated) != "Berlin" {
		t.Fatalf("location %q", LocationFromICS(updated))
	}
	f := EventFieldsFromICS(raw)
	if !f.AllDay || f.Location != "Athens" || f.Description != "Bring laptop" {
		t.Fatalf("event fields %+v", f)
	}
	s, e := NormalizeEventTimes("2026-08-28", "", true)
	if s != "20260828" || e != "20260829" {
		t.Fatalf("normalize %s %s", s, e)
	}
	s, e = NormalizeEventTimes("20260828", "20260830", true)
	if s != "20260828" || e != "20260831" {
		t.Fatalf("inclusive end %s %s", s, e)
	}
}

func TestCompactICSTime(t *testing.T) {
	cases := map[string]string{
		"2026-08-28T15:04":     "20260828T150400",
		"2026-08-28T15:04:05":  "20260828T150405",
		"2026-08-28T15:04:05Z": "20260828T150405Z",
		"20260828T090000Z":     "20260828T090000Z",
		"20260828T090000":      "20260828T090000",
		"20260828150400":       "20260828T150400",
		"2026-08-28":           "20260828",
		"20260828":             "20260828",
		"":                     "",
	}
	for in, want := range cases {
		if got := CompactICSTime(in); got != want {
			t.Errorf("CompactICSTime(%q)=%q want %q", in, got, want)
		}
	}
	if _, ok := ParseICSTime("2026-08-28T15:04:05Z"); !ok {
		t.Fatal("ParseICSTime should accept ISO datetime")
	}
	if _, ok := ParseICSTime("20260828150400"); !ok {
		t.Fatal("ParseICSTime should accept compact stamps missing T")
	}
}
