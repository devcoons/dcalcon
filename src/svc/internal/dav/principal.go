package dav

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
)

func (h *Handler) servePrincipal(w http.ResponseWriter, r *http.Request) {
	pr, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = h.Store.EnsureSchedulingCollections(r.Context(), pr.ID)

	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("DAV", "1, 3, access-control, calendar-access, addressbook, calendar-auto-schedule")
		w.Header().Set("Allow", "OPTIONS, PROPFIND")
		w.WriteHeader(http.StatusNoContent)
		return
	case "PROPFIND":
		h.principalPropfind(w, r, pr)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func wantsProp(body []byte, name string) bool {
	if len(bytes.TrimSpace(body)) == 0 || bytes.Contains(bytes.ToLower(body), []byte("allprop")) {
		return true
	}
	return bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(name)))
}

func (h *Handler) principalPropfind(w http.ResponseWriter, r *http.Request, pr auth.Principal) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	u, err := h.Store.UserByID(r.Context(), pr.ID)
	if err != nil {
		http.Error(w, "user", http.StatusInternalServerError)
		return
	}
	principal := davpath.PrincipalPath(pr.Username)
	calHome := davpath.CalendarHome(pr.Username)
	abHome := davpath.AddressBookHome(pr.Username)
	inbox := davpath.CalendarPath(pr.Username, "inbox")
	outbox := davpath.CalendarPath(pr.Username, "outbox")
	display := u.DisplayName
	if display == "" {
		display = u.Username
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:CR="urn:ietf:params:xml:ns:carddav" xmlns:CS="http://calendarserver.org/ns/">`)
	b.WriteString(`<D:response><D:href>`)
	davpath.WriteXML(&b, principal)
	b.WriteString(`</D:href><D:propstat><D:prop>`)
	if wantsProp(body, "resourcetype") {
		b.WriteString(`<D:resourcetype><D:principal/></D:resourcetype>`)
	}
	if wantsProp(body, "displayname") {
		b.WriteString(`<D:displayname>`)
		davpath.WriteXML(&b, display)
		b.WriteString(`</D:displayname>`)
	}
	if wantsProp(body, "current-user-principal") {
		b.WriteString(`<D:current-user-principal><D:href>`)
		davpath.WriteXML(&b, principal)
		b.WriteString(`</D:href></D:current-user-principal>`)
	}
	if wantsProp(body, "calendar-home-set") {
		b.WriteString(`<C:calendar-home-set><D:href>`)
		davpath.WriteXML(&b, calHome)
		b.WriteString(`</D:href></C:calendar-home-set>`)
	}
	if wantsProp(body, "addressbook-home-set") {
		b.WriteString(`<CR:addressbook-home-set><D:href>`)
		davpath.WriteXML(&b, abHome)
		b.WriteString(`</D:href></CR:addressbook-home-set>`)
	}
	if wantsProp(body, "schedule-inbox-URL") {
		b.WriteString(`<C:schedule-inbox-URL><D:href>`)
		davpath.WriteXML(&b, inbox)
		b.WriteString(`</D:href></C:schedule-inbox-URL>`)
	}
	if wantsProp(body, "schedule-outbox-URL") {
		b.WriteString(`<C:schedule-outbox-URL><D:href>`)
		davpath.WriteXML(&b, outbox)
		b.WriteString(`</D:href></C:schedule-outbox-URL>`)
	}
	if wantsProp(body, "calendar-user-address-set") {
		b.WriteString(`<C:calendar-user-address-set><D:href>mailto:`)
		davpath.WriteXML(&b, schedule.LocalMailbox(u.Username))
		b.WriteString(`</D:href>`)
		if u.Email != "" && !strings.EqualFold(u.Email, schedule.LocalMailbox(u.Username)) {
			b.WriteString(`<D:href>mailto:`)
			davpath.WriteXML(&b, u.Email)
			b.WriteString(`</D:href>`)
		}
		b.WriteString(`<D:href>`)
		davpath.WriteXML(&b, principal)
		b.WriteString(`</D:href></C:calendar-user-address-set>`)
	}
	if wantsProp(body, "calendar-user-type") {
		b.WriteString(`<C:calendar-user-type>INDIVIDUAL</C:calendar-user-type>`)
	}
	readHrefs, writeHrefs := proxyShareHrefs(r, h, pr)
	if wantsProp(body, "calendar-proxy-read-for") {
		b.WriteString(`<CS:calendar-proxy-read-for>`)
		for _, href := range readHrefs {
			b.WriteString(`<D:href>`)
			davpath.WriteXML(&b, href)
			b.WriteString(`</D:href>`)
		}
		b.WriteString(`</CS:calendar-proxy-read-for>`)
	}
	if wantsProp(body, "calendar-proxy-write-for") {
		b.WriteString(`<CS:calendar-proxy-write-for>`)
		for _, href := range writeHrefs {
			b.WriteString(`<D:href>`)
			davpath.WriteXML(&b, href)
			b.WriteString(`</D:href>`)
		}
		b.WriteString(`</CS:calendar-proxy-write-for>`)
	}
	if wantsProp(body, "principal-collection-set") {
		b.WriteString(`<D:principal-collection-set><D:href>`)
		davpath.WriteXML(&b, davpath.PrincipalsRoot)
		b.WriteString(`</D:href></D:principal-collection-set>`)
	}
	b.WriteString(`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("DAV", "1, 3, access-control, calendar-access, addressbook, calendar-auto-schedule")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(b.String()))
}

func proxyShareHrefs(r *http.Request, h *Handler, pr auth.Principal) (read, write []string) {
	list, err := h.Store.ListCalendars(r.Context(), pr.ID)
	if err != nil {
		return nil, nil
	}
	for _, c := range list {
		if !c.Shared {
			continue
		}
		href := davpath.CalendarPath(pr.Username, storage.ShareSlug(c.ID))
		if c.Access == "write" {
			write = append(write, href)
		} else {
			read = append(read, href)
		}
	}
	return read, write
}
