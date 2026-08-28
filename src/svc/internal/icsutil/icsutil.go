package icsutil

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
)

func ETag(body string) string {
	sum := sha256.Sum256([]byte(body))
	return fmt.Sprintf("%x", sum[:8])
}

func CalendarUID(cal *ical.Calendar) string {
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			continue
		}
		if uid, err := c.Props.Text(ical.PropUID); err == nil && uid != "" {
			return uid
		}
	}
	return ""
}

func CalendarComponent(cal *ical.Calendar) string {
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			continue
		}
		return c.Name
	}
	return ical.CompEvent
}

func CalendarComponentFromICS(raw string) string {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return ical.CompEvent
	}
	return CalendarComponent(cal)
}

func CalendarSummary(cal *ical.Calendar) string {
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			continue
		}
		if s, err := c.Props.Text(ical.PropSummary); err == nil {
			return s
		}
	}
	return ""
}

func SummaryFromICS(raw string) string {
	cal, err := ParseCalendar(raw)
	if err != nil {
		return ""
	}
	return CalendarSummary(cal)
}

func vcardLineValue(raw, prefix string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		u := strings.ToUpper(line)
		if strings.HasPrefix(u, prefix) {
			_, rest, ok := strings.Cut(line, ":")
			if ok {
				return strings.TrimSpace(rest)
			}
		}
	}
	return ""
}

func VCardEmail(raw string) string { return vcardLineValue(raw, "EMAIL") }
func VCardTel(raw string) string   { return vcardLineValue(raw, "TEL") }

func CalendarRange(cal *ical.Calendar) (start, end string) {
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			continue
		}
		if p := c.Props.Get(ical.PropDateTimeStart); p != nil {
			start = strings.TrimSpace(p.Value)
		}
		if p := c.Props.Get(ical.PropDateTimeEnd); p != nil {
			end = strings.TrimSpace(p.Value)
		}
		return start, end
	}
	return "", ""
}

func EncodeCalendar(cal *ical.Calendar) (string, error) {
	var b strings.Builder
	if err := ical.NewEncoder(&b).Encode(cal); err != nil {
		return "", err
	}
	return b.String(), nil
}

func ParseCalendar(raw string) (*ical.Calendar, error) {
	return ical.NewDecoder(strings.NewReader(raw)).Decode()
}

func EncodeCard(card vcard.Card) (string, error) {
	var b strings.Builder
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		return "", err
	}
	return b.String(), nil
}

func ParseCard(raw string) (vcard.Card, error) {
	return vcard.NewDecoder(strings.NewReader(raw)).Decode()
}

func CardFN(card vcard.Card) string {
	if v := card.PreferredValue(vcard.FieldFormattedName); v != "" {
		return v
	}
	if n := card.Name(); n != nil {
		return strings.TrimSpace(strings.Join([]string{n.GivenName, n.FamilyName}, " "))
	}
	return ""
}

func CardBDAY(card vcard.Card) string {
	return card.PreferredValue(vcard.FieldBirthday)
}

func CardAnniversary(card vcard.Card) string {
	if v := card.PreferredValue("ANNIVERSARY"); v != "" {
		return v
	}
	return card.PreferredValue("X-ANNIVERSARY")
}

func CardUID(card vcard.Card) string {
	return card.PreferredValue(vcard.FieldUID)
}

func NewEventICS(uid, summary, dtstart, dtend, description, location string) string {
	now := time.Now().UTC().Format("20060102T150405Z")
	b := strings.Builder{}
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//EN\r\nCALSCALE:GREGORIAN\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	fmt.Fprintf(&b, "UID:%s\r\n", uid)
	fmt.Fprintf(&b, "DTSTAMP:%s\r\n", now)
	if len(dtstart) == 8 {
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", dtstart)
	} else {
		fmt.Fprintf(&b, "DTSTART:%s\r\n", dtstart)
	}
	if dtend != "" {
		if len(dtend) == 8 {
			fmt.Fprintf(&b, "DTEND;VALUE=DATE:%s\r\n", dtend)
		} else {
			fmt.Fprintf(&b, "DTEND:%s\r\n", dtend)
		}
	}
	fmt.Fprintf(&b, "SUMMARY:%s\r\n", escapeICS(summary))
	if description != "" {
		fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", escapeICS(description))
	}
	if location != "" {
		fmt.Fprintf(&b, "LOCATION:%s\r\n", escapeICS(location))
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
