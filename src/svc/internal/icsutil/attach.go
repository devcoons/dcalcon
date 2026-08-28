package icsutil

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/emersion/go-ical"
)

type InlineAttach struct {
	Filename    string
	ContentType string
	Data        []byte
}

type AttachSplit struct {
	ICS        string
	Inlines    []InlineAttach
	External   []ical.Prop
	ManagedIDs []string
}

type ManagedAttach struct {
	PublicID    string
	Filename    string
	ContentType string
	Size        int64
}

func SplitAttachments(raw string, calendarID int64) (AttachSplit, error) {
	var out AttachSplit
	cal, err := ParseCalendar(raw)
	if err != nil {
		return out, err
	}
	host := attachHost(cal)
	if host == nil {
		out.ICS = raw
		return out, nil
	}
	props := host.Props.Values(ical.PropAttach)
	host.Props.Del(ical.PropAttach)
	seen := map[string]bool{}
	for i := range props {
		p := props[i]
		if inline, ok := inlineAttach(p); ok {
			out.Inlines = append(out.Inlines, inline)
			continue
		}
		id := managedID(p, calendarID)
		if id != "" && !seen[id] {
			seen[id] = true
			out.ManagedIDs = append(out.ManagedIDs, id)
			continue
		}
		out.External = append(out.External, p)
	}
	encoded, err := EncodeCalendar(cal)
	if err != nil {
		return out, err
	}
	out.ICS = encoded
	return out, nil
}

func WriteAttachments(split AttachSplit, managed []ManagedAttach, publicURL string, calendarID int64) (string, error) {
	cal, err := ParseCalendar(split.ICS)
	if err != nil {
		return "", err
	}
	host := attachHost(cal)
	if host == nil {
		return split.ICS, nil
	}
	host.Props.Del(ical.PropAttach)
	for i := range split.External {
		p := split.External[i]
		host.Props.Add(&p)
	}
	base := strings.TrimRight(publicURL, "/")
	for _, a := range managed {
		p := ical.NewProp(ical.PropAttach)
		p.Value = AttachmentURI(base, a.PublicID)
		p.Params.Set("MANAGED-ID", a.PublicID)
		if a.ContentType != "" {
			p.Params.Set("FMTTYPE", a.ContentType)
		}
		if a.Filename != "" {
			p.Params.Set("FILENAME", a.Filename)
		}
		p.Params.Set("SIZE", strconv.FormatInt(a.Size, 10))
		host.Props.Add(p)
	}
	_ = calendarID
	return EncodeCalendar(cal)
}

func AttachmentURI(publicURL, publicID string) string {
	p := "/dav/attachments/" + publicID
	if publicURL == "" {
		return p
	}
	return strings.TrimRight(publicURL, "/") + p
}

func attachHost(cal *ical.Calendar) *ical.Component {
	var fallback *ical.Component
	for _, c := range cal.Children {
		if c.Name == ical.CompTimezone {
			continue
		}
		if fallback == nil {
			fallback = c
		}
		if len(c.Props.Values(ical.PropAttach)) > 0 {
			return c
		}
	}
	return fallback
}

func inlineAttach(p ical.Prop) (InlineAttach, bool) {
	valueType := strings.ToUpper(p.Params.Get("VALUE"))
	enc := strings.ToUpper(p.Params.Get("ENCODING"))
	isBin := valueType == "BINARY" || enc == "BASE64"
	if !isBin {
		v := strings.TrimSpace(p.Value)
		if strings.Contains(v, "://") || strings.HasPrefix(v, "/") || strings.HasPrefix(strings.ToLower(v), "cid:") {
			return InlineAttach{}, false
		}
		if v == "" {
			return InlineAttach{}, false
		}
	}
	data, err := p.Binary()
	if err != nil {
		data, err = base64.StdEncoding.DecodeString(strings.TrimSpace(p.Value))
		if err != nil || len(data) == 0 {
			return InlineAttach{}, false
		}
	}
	name := p.Params.Get("FILENAME")
	if name == "" {
		name = p.Params.Get("NAME")
	}
	if name == "" {
		name = "attachment"
	}
	ct := p.Params.Get("FMTTYPE")
	if ct == "" {
		ct = p.Params.Get("FMTYPE")
	}
	return InlineAttach{Filename: name, ContentType: ct, Data: data}, true
}

func managedID(p ical.Prop, calendarID int64) string {
	if id := strings.TrimSpace(p.Params.Get("MANAGED-ID")); looksLikeID(id) {
		return id
	}
	return managedIDFromURI(p.Value, calendarID)
}

func managedIDFromURI(uri string, calendarID int64) string {
	s := strings.TrimSpace(uri)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		s = u.Path
	}
	if id := strings.TrimPrefix(s, "/dav/attachments/"); id != s {
		id = strings.Trim(strings.SplitN(id, "?", 2)[0], "/")
		if looksLikeID(id) {
			return id
		}
	}
	marker := fmt.Sprintf("/api/v1/calendars/%d/attachments/", calendarID)
	if i := strings.Index(s, marker); i >= 0 {
		id := strings.Trim(strings.SplitN(s[i+len(marker):], "?", 2)[0], "/")
		if looksLikeID(id) {
			return id
		}
	}
	if base := path.Base(s); looksLikeID(base) && strings.Contains(s, "/attachments/") {
		return base
	}
	return ""
}

func looksLikeID(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}
