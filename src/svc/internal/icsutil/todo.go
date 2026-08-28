package icsutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

func NewTodoICS(uid, summary, due, description, status string) string {
	now := time.Now().UTC().Format("20060102T150405Z")
	if status == "" {
		status = "NEEDS-ACTION"
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nCALSCALE:GREGORIAN\r\n")
	b.WriteString("BEGIN:VTODO\r\n")
	fmt.Fprintf(&b, "UID:%s\r\n", uid)
	fmt.Fprintf(&b, "DTSTAMP:%s\r\n", now)
	fmt.Fprintf(&b, "SUMMARY:%s\r\n", escapeICS(summary))
	if due != "" {
		if len(due) == 8 {
			fmt.Fprintf(&b, "DUE;VALUE=DATE:%s\r\n", due)
		} else {
			fmt.Fprintf(&b, "DUE:%s\r\n", due)
		}
	}
	if description != "" {
		fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", escapeICS(description))
	}
	fmt.Fprintf(&b, "STATUS:%s\r\n", status)
	if strings.EqualFold(status, "COMPLETED") {
		fmt.Fprintf(&b, "PERCENT-COMPLETE:100\r\nCOMPLETED:%s\r\n", now)
	}
	b.WriteString("END:VTODO\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func UpdateTodoICS(raw, summary, due, description, status string) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	ev := firstTodo(cal)
	if ev == nil {
		return "", fmt.Errorf("not a task")
	}
	if strings.TrimSpace(summary) != "" {
		ev.Props.SetText(ical.PropSummary, summary)
	}
	if due != "" {
		setDateProp(ev, ical.PropDue, due)
	} else {
		ev.Props.Del(ical.PropDue)
	}
	if description != "" {
		ev.Props.SetText(ical.PropDescription, description)
	} else {
		ev.Props.Del(ical.PropDescription)
	}
	if status == "" {
		status = "NEEDS-ACTION"
	}
	ev.Props.SetText(ical.PropStatus, status)
	if strings.EqualFold(status, "COMPLETED") {
		ev.Props.SetText("PERCENT-COMPLETE", "100")
		ev.Props.SetDateTime(ical.PropCompleted, time.Now().UTC())
	} else {
		ev.Props.Del("PERCENT-COMPLETE")
		ev.Props.Del(ical.PropCompleted)
	}
	ev.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	return EncodeCalendar(cal)
}

func TodoStatusFromICS(raw string) string {
	return TodoFieldsFromICS(raw).Status
}

func DueFromICS(raw string) string {
	return TodoFieldsFromICS(raw).Due
}

type TodoFields struct {
	Status      string
	Due         string
	Description string
}

func TodoFieldsFromICS(raw string) TodoFields {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return TodoFields{}
	}
	return TodoFieldsFromCal(cal)
}

func TodoFieldsFromCal(cal *ical.Calendar) TodoFields {
	ev := firstTodo(cal)
	if ev == nil {
		return TodoFields{}
	}
	f := TodoFields{Description: CalendarDescription(cal)}
	s, _ := ev.Props.Text(ical.PropStatus)
	f.Status = s
	if p := ev.Props.Get(ical.PropDue); p != nil {
		f.Due = strings.TrimSpace(p.Value)
	}
	return f
}

func firstTodo(cal *ical.Calendar) *ical.Component {
	for _, c := range cal.Children {
		if strings.EqualFold(c.Name, "VTODO") {
			return c
		}
	}
	return nil
}

type BusyPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func BusyPeriods(raw string, start, end time.Time) []BusyPeriod {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return nil
	}
	if !isOpaqueBusyCal(cal) {
		return nil
	}
	exp := ExpandCalendar(cal, start, end)
	if exp == nil {
		return nil
	}
	var out []BusyPeriod
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
		ot, ok := ParseICSTime(ds)
		if !ok {
			continue
		}
		otEnd, okEnd := ParseICSTime(de)
		if !okEnd || otEnd.IsZero() {
			if len(ds) == 8 {
				otEnd = ot.Add(24 * time.Hour)
			} else {
				otEnd = ot.Add(time.Hour)
			}
		}
		if !rangeOverlap(ot, otEnd, start, end) {
			continue
		}
		out = append(out, BusyPeriod{Start: formatUTC(ot), End: formatUTC(otEnd)})
	}
	return out
}

func formatUTC(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

func EncodeFreeBusy(uid, organizer string, start, end time.Time, periods []BusyPeriod) string {
	now := time.Now().UTC().Format("20060102T150405Z")
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nMETHOD:REPLY\r\n")
	b.WriteString("BEGIN:VFREEBUSY\r\n")
	fmt.Fprintf(&b, "UID:%s\r\n", uid)
	fmt.Fprintf(&b, "DTSTAMP:%s\r\n", now)
	fmt.Fprintf(&b, "DTSTART:%s\r\n", formatUTC(start))
	fmt.Fprintf(&b, "DTEND:%s\r\n", formatUTC(end))
	if organizer != "" {
		fmt.Fprintf(&b, "ORGANIZER:%s\r\n", Mailto(organizer))
	}
	for _, p := range periods {
		fmt.Fprintf(&b, "FREEBUSY;FBTYPE=BUSY:%s/%s\r\n", p.Start, p.End)
	}
	b.WriteString("END:VFREEBUSY\r\nEND:VCALENDAR\r\n")
	return b.String()
}
