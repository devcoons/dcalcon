package icsutil

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

func CalendarDescription(cal *ical.Calendar) string {
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			continue
		}
		if s, err := c.Props.Text(ical.PropDescription); err == nil {
			return s
		}
	}
	return ""
}

func DescriptionFromICS(raw string) string {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return ""
	}
	return CalendarDescription(cal)
}

func firstEvent(cal *ical.Calendar) *ical.Component {
	for _, c := range cal.Children {
		if c.Name == ical.CompEvent {
			return c
		}
	}
	for _, c := range cal.Children {
		if c.Name != ical.CompTimezone {
			return c
		}
	}
	return nil
}

func setDateProp(comp *ical.Component, name, value string) {
	if strings.TrimSpace(value) == "" {
		comp.Props.Del(name)
		return
	}
	value = CompactICSTime(value)
	old := comp.Props.Get(name)
	if old != nil && dateValuesEqual(old.Value, value) {
		return
	}
	p := ical.NewProp(name)
	copyDateParams(p, old, value)
	p.Value = value
	comp.Props.Set(p)
}

func dateValuesEqual(a, b string) bool {
	return CompactICSTime(a) == CompactICSTime(b)
}

func icsIsDate(value string) bool {
	value = CompactICSTime(value)
	return len(value) == 8 && !strings.Contains(value, "T")
}

func icsIsUTC(value string) bool {
	return strings.HasSuffix(strings.ToUpper(CompactICSTime(value)), "Z")
}

func copyDateParams(dst, src *ical.Prop, value string) {
	if dst.Params == nil {
		dst.Params = make(ical.Params)
	}
	if icsIsDate(value) {
		dst.Params.Set("VALUE", "DATE")
	}
	if src == nil {
		return
	}
	date, utc := icsIsDate(value), icsIsUTC(value)
	for k, vs := range src.Params {
		ku := strings.ToUpper(k)
		if ku == "VALUE" {
			continue
		}
		if ku == "TZID" && (utc || date) {
			continue
		}
		for _, v := range vs {
			dst.Params.Add(k, v)
		}
	}
}

func UpdateEventICS(raw, summary, dtstart, dtend, description, location string) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	ev := firstEvent(cal)
	if ev == nil {
		return NewEventICS("", summary, dtstart, dtend, description, location), nil
	}
	if strings.TrimSpace(summary) != "" {
		ev.Props.SetText(ical.PropSummary, summary)
	}
	if strings.TrimSpace(dtstart) != "" {
		setDateProp(ev, ical.PropDateTimeStart, dtstart)
	}
	if dtend != "" {
		setDateProp(ev, ical.PropDateTimeEnd, dtend)
	} else {
		ev.Props.Del(ical.PropDateTimeEnd)
	}
	if description != "" {
		ev.Props.SetText(ical.PropDescription, description)
	} else {
		ev.Props.Del(ical.PropDescription)
	}
	if strings.TrimSpace(location) != "" {
		ev.Props.SetText(ical.PropLocation, location)
	} else {
		ev.Props.Del(ical.PropLocation)
	}
	ev.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	return EncodeCalendar(cal)
}

func LocationFromICS(raw string) string {
	return EventFieldsFromICS(raw).Location
}

func AllDayFromICS(raw string) bool {
	return EventFieldsFromICS(raw).AllDay
}

type EventFields struct {
	Description  string
	Location     string
	AllDay       bool
	RRule        string
	AlarmMinutes int
	Attendees    []AttendeeInfo
}

func EventFieldsFromICS(raw string) EventFields {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return EventFields{}
	}
	return EventFieldsFromCal(cal)
}

func EventFieldsFromCal(cal *ical.Calendar) EventFields {
	f := EventFields{Description: CalendarDescription(cal)}
	ev := firstEvent(cal)
	if ev == nil {
		return f
	}
	if s, _ := ev.Props.Text(ical.PropLocation); s != "" {
		f.Location = s
	}
	if p := ev.Props.Get(ical.PropDateTimeStart); p != nil {
		if strings.EqualFold(p.Params.Get("VALUE"), "DATE") {
			f.AllDay = true
		} else {
			f.AllDay = len(p.Value) == 8 && !strings.Contains(p.Value, "T")
		}
	}
	if p := ev.Props.Get(ical.PropRecurrenceRule); p != nil {
		f.RRule = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(p.Value), `\;`, `;`), `\,`, `,`)
	}
	for _, c := range ev.Children {
		if c.Name != ical.CompAlarm {
			continue
		}
		if p := c.Props.Get(ical.PropTrigger); p != nil {
			f.AlarmMinutes = parseTriggerMinutes(p.Value)
			break
		}
	}
	for _, p := range ev.Props.Values(ical.PropAttendee) {
		f.Attendees = append(f.Attendees, AttendeeInfo{
			Value:    p.Value,
			CN:       p.Params.Get("CN"),
			Partstat: p.Params.Get("PARTSTAT"),
		})
	}
	return f
}

func LocationFromCal(cal *ical.Calendar) string {
	return EventFieldsFromCal(cal).Location
}

func NormalizeEventTimes(start, end string, allDay bool) (string, string) {
	start = CompactICSTime(start)
	end = CompactICSTime(end)
	if !allDay {
		return start, end
	}
	start = dateOnly(start)
	if end == "" {
		return start, addOneDay(start)
	}
	end = dateOnly(end)
	// Form dates are inclusive; iCalendar DTEND is exclusive.
	if end <= start {
		end = addOneDay(start)
	} else {
		end = addOneDay(end)
	}
	return start, end
}

func CompactICSTime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	z := strings.HasSuffix(strings.ToUpper(v), "Z")
	v = strings.TrimSuffix(strings.TrimSuffix(v, "Z"), "z")
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, ":", "")
	v = strings.ReplaceAll(v, "T", "")
	v = strings.ReplaceAll(v, "t", "")
	if i := strings.IndexByte(v, '.'); i >= 0 {
		v = v[:i]
	}
	if len(v) == 12 {
		v += "00"
	}
	if len(v) == 8 {
		return v
	}
	if len(v) >= 14 {
		out := v[:8] + "T" + v[8:14]
		if z {
			out += "Z"
		}
		return out
	}
	if z {
		return v + "Z"
	}
	return v
}

func dateOnly(v string) string {
	v = CompactICSTime(v)
	if len(v) >= 8 {
		return v[:8]
	}
	return v
}

func addOneDay(yyyymmdd string) string {
	if len(yyyymmdd) < 8 {
		return yyyymmdd
	}
	t, err := time.Parse("20060102", yyyymmdd[:8])
	if err != nil {
		return yyyymmdd
	}
	return t.AddDate(0, 0, 1).Format("20060102")
}

func StripMethod(raw string) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return raw, err
	}
	cal.Props.Del(ical.PropMethod)
	return EncodeCalendar(cal)
}

func WithMethod(raw, method string) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	cal.Props.SetText(ical.PropMethod, method)
	return EncodeCalendar(cal)
}

func OrganizerValue(raw string) string {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return ""
	}
	ev := firstEvent(cal)
	if ev == nil {
		return ""
	}
	if p := ev.Props.Get(ical.PropOrganizer); p != nil {
		return p.Value
	}
	return ""
}

type AttendeeInfo struct {
	Value    string `json:"value"`
	CN       string `json:"cn"`
	Partstat string `json:"partstat"`
}

func Attendees(raw string) []AttendeeInfo {
	return EventFieldsFromICS(raw).Attendees
}

func Mailto(email string) string {
	email = strings.TrimSpace(email)
	if strings.HasPrefix(strings.ToLower(email), "mailto:") {
		return email
	}
	return "mailto:" + email
}

func AddrOf(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "mailto:")
	v = strings.TrimPrefix(v, "MAILTO:")
	return v
}

func SetOrganizerAttendees(raw, organizerEmail, organizerCN string, attendees []AttendeeInfo) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	ev := firstEvent(cal)
	if ev == nil {
		return raw, nil
	}
	org := ical.NewProp(ical.PropOrganizer)
	org.Value = Mailto(organizerEmail)
	if organizerCN != "" {
		org.Params.Set("CN", organizerCN)
	}
	ev.Props.Set(org)
	ev.Props.Del(ical.PropAttendee)
	for _, a := range attendees {
		p := ical.NewProp(ical.PropAttendee)
		p.Value = Mailto(a.Value)
		if a.CN != "" {
			p.Params.Set("CN", a.CN)
		}
		part := a.Partstat
		if part == "" {
			part = "NEEDS-ACTION"
		}
		p.Params.Set("PARTSTAT", part)
		p.Params.Set("RSVP", "TRUE")
		p.Params.Set("ROLE", "REQ-PARTICIPANT")
		ev.Props.Add(p)
	}
	return EncodeCalendar(cal)
}

func MergeOrganizerAttendees(raw, organizerEmail, organizerCN string, add []AttendeeInfo) (string, error) {
	seen := map[string]bool{}
	var merged []AttendeeInfo
	for _, a := range Attendees(raw) {
		k := strings.ToLower(AddrOf(a.Value))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		merged = append(merged, a)
	}
	for _, a := range add {
		k := strings.ToLower(AddrOf(a.Value))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		merged = append(merged, a)
	}
	return SetOrganizerAttendees(raw, organizerEmail, organizerCN, merged)
}

func SetAttendeePartstat(raw, attendeeEmail, partstat string) (string, error) {
	return SetAttendeePartstatAny(raw, []string{attendeeEmail}, partstat)
}

func SetAttendeePartstatAny(raw string, emails []string, partstat string) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	ev := firstEvent(cal)
	if ev == nil {
		return raw, nil
	}
	want := map[string]bool{}
	for _, e := range emails {
		if a := strings.ToLower(AddrOf(e)); a != "" {
			want[a] = true
		}
	}
	props := ev.Props.Values(ical.PropAttendee)
	ev.Props.Del(ical.PropAttendee)
	for i := range props {
		p := props[i]
		if want[strings.ToLower(AddrOf(p.Value))] {
			p.Params.Set("PARTSTAT", partstat)
		}
		ev.Props.Add(&p)
	}
	return EncodeCalendar(cal)
}
