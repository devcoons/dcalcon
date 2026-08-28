package icsutil

import (
	"strconv"
	"strings"

	"github.com/emersion/go-ical"
)

func MethodFromICS(raw string) string {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return ""
	}
	s, _ := cal.Props.Text(ical.PropMethod)
	return strings.ToUpper(strings.TrimSpace(s))
}

func RRuleFromICS(raw string) string {
	return EventFieldsFromICS(raw).RRule
}

func SetRRule(raw, rrule string) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	ev := firstEvent(cal)
	if ev == nil {
		return raw, nil
	}
	existing := ""
	if p := ev.Props.Get(ical.PropRecurrenceRule); p != nil {
		existing = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(p.Value), `\;`, `;`), `\,`, `,`)
	}
	merged := mergeRRule(existing, rrule)
	if merged == "" {
		ev.Props.Del(ical.PropRecurrenceRule)
	} else if existing != merged {
		p := ical.NewProp(ical.PropRecurrenceRule)
		p.Value = merged
		ev.Props.Set(p)
	}
	return EncodeCalendar(cal)
}

func parseRRuleParts(s string) [][2]string {
	var parts [][2]string
	for _, p := range strings.Split(s, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		parts = append(parts, [2]string{k, v})
	}
	return parts
}

func rrulePartMap(parts [][2]string) map[string]string {
	m := make(map[string]string, len(parts))
	for _, p := range parts {
		m[p[0]] = p[1]
	}
	return m
}

func mergeUNTIL(existing, incoming string) string {
	incoming = strings.TrimSpace(incoming)
	existing = strings.TrimSpace(existing)
	if incoming == "" {
		return existing
	}
	if existing == "" || incoming == existing {
		return incoming
	}
	if len(incoming) == 8 && strings.HasPrefix(existing, incoming) {
		return existing
	}
	return incoming
}

// mergeRRule overlays dashboard FREQ/INTERVAL/UNTIL/COUNT onto a client rule.
// Same FREQ keeps BYDAY, WKST, and other parts the UI does not edit.
func mergeRRule(existing, incoming string) string {
	incoming = strings.TrimSpace(incoming)
	existing = strings.TrimSpace(existing)
	if incoming == "" {
		return ""
	}
	if existing == "" || existing == incoming {
		return incoming
	}
	ex := parseRRuleParts(existing)
	in := parseRRuleParts(incoming)
	exm, inm := rrulePartMap(ex), rrulePartMap(in)
	if !strings.EqualFold(exm["FREQ"], inm["FREQ"]) || inm["FREQ"] == "" {
		return incoming
	}
	interval := "1"
	if v := inm["INTERVAL"]; v != "" {
		interval = v
	}
	type kv struct{ k, v string }
	var out []kv
	seen := map[string]bool{}
	push := func(k, v string) {
		if k == "" || seen[k] {
			return
		}
		if k == "INTERVAL" && (v == "" || v == "1") {
			seen[k] = true
			return
		}
		if v == "" {
			return
		}
		out = append(out, kv{k, v})
		seen[k] = true
	}
	for _, p := range ex {
		k, v := p[0], p[1]
		switch k {
		case "INTERVAL":
			v = interval
		case "UNTIL":
			if iv, ok := inm["UNTIL"]; ok {
				v = mergeUNTIL(v, iv)
			} else {
				continue
			}
		case "COUNT":
			if iv, ok := inm["COUNT"]; ok {
				v = iv
			} else {
				continue
			}
		default:
			if iv, ok := inm[k]; ok {
				v = iv
			}
		}
		push(k, v)
	}
	for _, p := range in {
		switch p[0] {
		case "INTERVAL":
			push("INTERVAL", interval)
		case "UNTIL":
			push("UNTIL", mergeUNTIL(exm["UNTIL"], p[1]))
		default:
			push(p[0], p[1])
		}
	}
	if !seen["INTERVAL"] {
		push("INTERVAL", interval)
	}
	var b strings.Builder
	for i, p := range out {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(p.v)
	}
	return b.String()
}

func AlarmMinutesFromICS(raw string) int {
	return EventFieldsFromICS(raw).AlarmMinutes
}

func parseTriggerMinutes(v string) int {
	v = strings.ToUpper(strings.TrimSpace(v))
	v = strings.TrimPrefix(v, "-P")
	v = strings.TrimPrefix(v, "-PT")
	v = strings.TrimPrefix(v, "P")
	v = strings.TrimPrefix(v, "T")
	n := 0
	num := 0
	for _, r := range v {
		if r >= '0' && r <= '9' {
			num = num*10 + int(r-'0')
			continue
		}
		switch r {
		case 'D':
			n += num * 24 * 60
		case 'H':
			n += num * 60
		case 'M':
			n += num
		case 'W':
			n += num * 7 * 24 * 60
		}
		num = 0
	}
	return n
}

func SetDisplayAlarm(raw string, minutesBefore int) (string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return "", err
	}
	ev := firstEvent(cal)
	if ev == nil {
		return raw, nil
	}
	kept := ev.Children[:0]
	for _, c := range ev.Children {
		if c.Name != ical.CompAlarm {
			kept = append(kept, c)
		}
	}
	ev.Children = kept
	if minutesBefore > 0 {
		al := ical.NewComponent(ical.CompAlarm)
		al.Props.SetText(ical.PropAction, "DISPLAY")
		al.Props.SetText("DESCRIPTION", "Reminder")
		trig := ical.NewProp(ical.PropTrigger)
		trig.Value = formatTrigger(minutesBefore)
		al.Props.Set(trig)
		ev.Children = append(ev.Children, al)
	}
	return EncodeCalendar(cal)
}

func formatTrigger(minutes int) string {
	if minutes%(24*60) == 0 {
		return "-P" + strconv.Itoa(minutes/(24*60)) + "D"
	}
	if minutes%60 == 0 {
		return "-PT" + strconv.Itoa(minutes/60) + "H"
	}
	return "-PT" + strconv.Itoa(minutes) + "M"
}

func IsOpaqueBusy(raw string) bool {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return false
	}
	return isOpaqueBusyCal(cal)
}

func isOpaqueBusyCal(cal *ical.Calendar) bool {
	ev := firstEvent(cal)
	if ev == nil || ev.Name != ical.CompEvent {
		return false
	}
	if st, _ := ev.Props.Text(ical.PropStatus); strings.EqualFold(st, "CANCELLED") {
		return false
	}
	if tr, _ := ev.Props.Text(ical.PropTransparency); strings.EqualFold(tr, "TRANSPARENT") {
		return false
	}
	return true
}

func SplitCalendarObjects(raw string) ([]string, error) {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return nil, err
	}
	var tzs, comps []*ical.Component
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			tzs = append(tzs, c)
			continue
		}
		comps = append(comps, c)
	}
	if len(comps) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(comps))
	for _, c := range comps {
		wrap := &ical.Calendar{Component: &ical.Component{
			Name:     ical.CompCalendar,
			Props:    cloneProps(cal.Props),
			Children: append(append([]*ical.Component{}, tzs...), c),
		}}
		wrap.Props.Del(ical.PropMethod)
		enc, err := EncodeCalendar(wrap)
		if err != nil {
			return nil, err
		}
		out = append(out, enc)
	}
	return out, nil
}

func JoinCalendars(raws []string) string {
	props := make(ical.Props)
	props.SetText(ical.PropVersion, "2.0")
	props.SetText(ical.PropProductID, "-//dCalCon//EN")
	var children []*ical.Component
	seenTZ := map[string]bool{}
	for _, raw := range raws {
		cal, err := ParseCalendar(raw)
		if err != nil {
			continue
		}
		for _, c := range cal.Children {
			if c.Name == ical.CompTimezone {
				tzid, _ := c.Props.Text("TZID")
				if tzid != "" && seenTZ[tzid] {
					continue
				}
				if tzid != "" {
					seenTZ[tzid] = true
				}
			}
			children = append(children, c)
		}
	}
	wrap := &ical.Calendar{Component: &ical.Component{
		Name:     ical.CompCalendar,
		Props:    props,
		Children: children,
	}}
	enc, err := EncodeCalendar(wrap)
	if err != nil {
		return ""
	}
	return enc
}
