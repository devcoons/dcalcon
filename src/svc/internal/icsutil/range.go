package icsutil

import (
	"time"

	"github.com/emersion/go-ical"
)

func OverlapsTimeRange(cal *ical.Calendar, start, end time.Time) bool {
	if cal == nil {
		return true
	}
	if start.IsZero() && end.IsZero() {
		return true
	}
	events := cal.Events()
	if len(events) == 0 {
		return simpleRangeOverlap(cal, start, end)
	}
	for _, ev := range events {
		if componentOverlaps(ev.Component, start, end) {
			return true
		}
	}
	return false
}

func simpleRangeOverlap(cal *ical.Calendar, start, end time.Time) bool {
	ds, de := CalendarRange(cal)
	return timesOverlap(ds, de, start, end)
}

func componentOverlaps(comp *ical.Component, start, end time.Time) bool {
	set, err := comp.RecurrenceSet(time.UTC)
	if err != nil || set == nil {
		ds, de := "", ""
		if p := comp.Props.Get(ical.PropDateTimeStart); p != nil {
			ds = p.Value
		}
		if p := comp.Props.Get(ical.PropDateTimeEnd); p != nil {
			de = p.Value
		}
		return timesOverlap(ds, de, start, end)
	}
	dtstart, err := comp.Props.DateTime(ical.PropDateTimeStart, time.UTC)
	if err != nil {
		return true
	}
	dur := time.Hour
	if p := comp.Props.Get(ical.PropDateTimeEnd); p != nil {
		if dtend, err := p.DateTime(time.UTC); err == nil && dtend.After(dtstart) {
			dur = dtend.Sub(dtstart)
		}
	} else if p := comp.Props.Get(ical.PropDateTimeStart); p != nil && len(p.Value) == 8 {
		dur = 24 * time.Hour
	}
	from := dtstart
	if !start.IsZero() {
		from = start.Add(-dur)
		if from.Before(dtstart) {
			from = dtstart.Add(-time.Second)
		}
	}
	to := end
	if to.IsZero() {
		to = from.Add(2 * 366 * 24 * time.Hour)
	}
	if to.Sub(from) > 3*366*24*time.Hour {
		to = from.Add(3 * 366 * 24 * time.Hour)
	}
	occ := set.Between(from, to, true)
	const capN = 500
	if len(occ) > capN {
		occ = occ[:capN]
	}
	for _, o := range occ {
		instEnd := o.Add(dur)
		if rangeOverlap(o, instEnd, start, end) {
			return true
		}
	}
	return false
}

func timesOverlap(ds, de string, start, end time.Time) bool {
	ot, ok := ParseICSTime(ds)
	if !ok {
		return true
	}
	var otEnd time.Time
	if de != "" {
		otEnd, _ = ParseICSTime(de)
	}
	if otEnd.IsZero() {
		if len(ds) == 8 {
			otEnd = ot.Add(24 * time.Hour)
		} else {
			otEnd = ot.Add(time.Hour)
		}
	}
	return rangeOverlap(ot, otEnd, start, end)
}

func rangeOverlap(ot, otEnd, start, end time.Time) bool {
	if !end.IsZero() && !ot.Before(end) {
		return false
	}
	if !start.IsZero() && !otEnd.After(start) {
		return false
	}
	return true
}

func ParseICSTime(s string) (time.Time, bool) {
	s = CompactICSTime(trimICS(s))
	layouts := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func trimICS(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
