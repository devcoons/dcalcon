package icsutil

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

const maxExpanded = 400

func ExpandCalendar(cal *ical.Calendar, start, end time.Time) *ical.Calendar {
	if cal == nil {
		return nil
	}
	if start.IsZero() && end.IsZero() {
		return cal
	}
	out := &ical.Calendar{Component: &ical.Component{
		Name:     cal.Name,
		Props:    cloneProps(cal.Props),
		Children: nil,
	}}
	for _, child := range cal.Children {
		if child == nil {
			continue
		}
		if child.Name != ical.CompEvent {
			out.Children = append(out.Children, child)
			continue
		}
		inst := expandEvent(child, start, end)
		if len(inst) == 0 {
			if componentOverlaps(child, start, end) {
				out.Children = append(out.Children, child)
			}
			continue
		}
		out.Children = append(out.Children, inst...)
	}
	return out
}

func expandEvent(comp *ical.Component, start, end time.Time) []*ical.Component {
	if comp.Props.Get(ical.PropRecurrenceRule) == nil && comp.Props.Get("RDATE") == nil {
		return nil
	}
	set, err := comp.RecurrenceSet(time.UTC)
	if err != nil || set == nil {
		return nil
	}
	dtstart, err := comp.Props.DateTime(ical.PropDateTimeStart, time.UTC)
	if err != nil {
		return nil
	}
	dur := time.Hour
	allDay := false
	if p := comp.Props.Get(ical.PropDateTimeStart); p != nil && len(strings.TrimSpace(p.Value)) == 8 {
		allDay = true
		dur = 24 * time.Hour
	}
	if p := comp.Props.Get(ical.PropDateTimeEnd); p != nil {
		if dtend, err := p.DateTime(time.UTC); err == nil && dtend.After(dtstart) {
			dur = dtend.Sub(dtstart)
		}
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
		to = from.Add(366 * 24 * time.Hour)
	}
	if to.Sub(from) > 3*366*24*time.Hour {
		to = from.Add(3 * 366 * 24 * time.Hour)
	}
	occ := set.Between(from, to, true)
	if len(occ) > maxExpanded {
		occ = occ[:maxExpanded]
	}
	out := make([]*ical.Component, 0, len(occ))
	for _, o := range occ {
		instEnd := o.Add(dur)
		if !rangeOverlap(o, instEnd, start, end) {
			continue
		}
		out = append(out, instanceOf(comp, o, dur, allDay))
	}
	return out
}

func instanceOf(src *ical.Component, when time.Time, dur time.Duration, allDay bool) *ical.Component {
	c := &ical.Component{
		Name:     src.Name,
		Props:    cloneProps(src.Props),
		Children: src.Children,
	}
	c.Props.Del(ical.PropRecurrenceRule)
	c.Props.Del("RDATE")
	c.Props.Del("EXDATE")
	setStamp := func(name string, t time.Time, asDate bool) {
		p := ical.NewProp(name)
		if asDate {
			p.Params.Set("VALUE", "DATE")
			p.Value = t.UTC().Format("20060102")
		} else {
			p.Value = t.UTC().Format("20060102T150405Z")
		}
		c.Props.Set(p)
	}
	setStamp(ical.PropDateTimeStart, when, allDay)
	setStamp(ical.PropDateTimeEnd, when.Add(dur), allDay)
	setStamp("RECURRENCE-ID", when, allDay)
	if c.Props.Get(ical.PropDateTimeStamp) == nil {
		setStamp(ical.PropDateTimeStamp, time.Now().UTC(), false)
	}
	return c
}

func cloneProps(in ical.Props) ical.Props {
	out := make(ical.Props, len(in))
	for k, list := range in {
		cp := make([]ical.Prop, len(list))
		copy(cp, list)
		out[k] = cp
	}
	return out
}

func FirstOccurrence(raw string, start, end time.Time) (dtstart, dtend string, ok bool) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", "", false
	}
	return FirstOccurrenceCal(cal, start, end)
}

func FirstOccurrenceCal(cal *ical.Calendar, start, end time.Time) (dtstart, dtend string, ok bool) {
	exp := ExpandCalendar(cal, start, end)
	if exp == nil {
		return "", "", false
	}
	bestStart, bestEnd := "", ""
	for _, c := range exp.Children {
		if c.Name != ical.CompEvent {
			continue
		}
		ds, de := "", ""
		if p := c.Props.Get(ical.PropDateTimeStart); p != nil {
			ds = strings.TrimSpace(p.Value)
		}
		if p := c.Props.Get(ical.PropDateTimeEnd); p != nil {
			de = strings.TrimSpace(p.Value)
		}
		if ds == "" {
			continue
		}
		if bestStart == "" || ds < bestStart {
			bestStart, bestEnd = ds, de
		}
	}
	return bestStart, bestEnd, bestStart != ""
}
