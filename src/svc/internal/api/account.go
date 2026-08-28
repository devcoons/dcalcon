package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
)

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	ctx := r.Context()
	u, err := h.Store.UserByID(ctx, p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	cals, err := h.Store.ListCalendars(ctx, p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	contacts, err := h.Store.CountContacts(ctx, p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	pendingN, err := h.Store.CountPendingInbox(ctx, p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids, err := h.Store.GetImportantDates(ctx, p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	accounts, err := h.Store.ListConnectedAccounts(ctx, p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	mailOn, mailAddr := false, ""
	for _, a := range accounts {
		if a.Status == "connected" {
			mailOn, mailAddr = true, a.Email
			break
		}
	}

	todayT := time.Now()
	today := todayT.Format("20060102")
	week := todayT.AddDate(0, 0, 7).Format("20060102")
	horizon := todayT.AddDate(0, 0, 21).Format("20060102")
	winStart, _ := time.ParseInLocation("20060102", today, time.Local)
	winEnd := winStart.AddDate(0, 0, 22)

	calList := make([]overviewCal, 0)
	upcoming := make([]overviewEvent, 0)
	n, shared, soon := 0, 0, 0
	for _, c := range cals {
		if c.Kind == "inbox" || c.Kind == "outbox" {
			continue
		}
		n++
		if c.Shared {
			shared++
		}
		label := c.Name
		if c.Shared && c.OwnerUsername != "" {
			label = c.OwnerUsername + " — " + c.Name
		}
		calList = append(calList, overviewCal{
			ID: c.ID, Name: c.Name, Color: c.Color, Kind: c.Kind,
			Shared: c.Shared, ReadOnly: c.ReadOnly, Access: c.Access, OwnerUsername: c.OwnerUsername,
		})
		objs, err := h.Store.ListCalendarObjectsByComponent(ctx, c.ID, "VEVENT")
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, o := range objs {
			ds, de := o.DTStart, o.DTEnd
			allDay, loc := false, ""
			if cal, err := icsutil.ParseCalendar(o.ICS); err == nil {
				f := icsutil.EventFieldsFromCal(cal)
				allDay, loc = f.AllDay, f.Location
				if strings.Contains(o.ICS, "RRULE") || strings.Contains(o.ICS, "RDATE") {
					if next, nextEnd, ok := icsutil.FirstOccurrenceCal(cal, winStart, winEnd); ok {
						ds, de = next, nextEnd
					}
				}
			}
			day := ds
			if len(day) > 8 {
				day = day[:8]
			}
			if day < today || day > horizon {
				continue
			}
			if day <= week {
				soon++
			}
			upcoming = append(upcoming, overviewEvent{
				Href: o.Href, CalendarID: c.ID, Summary: o.Summary,
				Location: loc,
				DTStart:  ds, DTEnd: de,
				AllDay: allDay,
				Color:  c.Color, CalendarName: label,
			})
		}
		todos, err := h.Store.ListCalendarObjectsByComponent(ctx, c.ID, "VTODO")
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, o := range todos {
			start, allDay, loc, ok := overviewTask(o)
			if !ok {
				continue
			}
			ds := start
			day := ds
			if len(day) > 8 {
				day = day[:8]
			}
			if day < today || day > horizon {
				continue
			}
			upcoming = append(upcoming, overviewEvent{
				Href: o.Href, CalendarID: c.ID, Summary: o.Summary,
				Location: loc,
				DTStart:  ds, DTEnd: ds,
				AllDay: allDay,
				Color:  c.Color, CalendarName: label,
				Kind: "task",
			})
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i].DTStart < upcoming[j].DTStart })
	if len(upcoming) > 8 {
		upcoming = upcoming[:8]
	}

	inbox, err := h.Store.ListPendingInbox(ctx, p.ID, 3)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	pending := make([]overviewInvite, 0, len(inbox))
	for _, it := range inbox {
		dto := toInvitationDTO(it)
		pending = append(pending, overviewInvite{
			ID: dto.ID, Summary: dto.Summary, Organizer: dto.Organizer, DTStart: dto.DTStart,
		})
	}

	httpx.JSON(w, http.StatusOK, overviewDTO{
		Calendars:             n,
		Contacts:              contacts,
		PendingInvitations:    pendingN,
		ImportantDatesEnabled: ids.Enabled,
		SharedCalendars:       shared,
		EventsSoon:            soon,
		MailConnected:         mailOn,
		MailAddress:           mailAddr,
		TotpEnabled:           u.TOTPEnabled,
		CalendarList:          calList,
		Upcoming:              upcoming,
		Pending:               pending,
	})
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	httpx.JSON(w, http.StatusOK, h.setupFor(p.Username))
}

func (h *Handler) patchMe(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	u, err := h.Store.UserByID(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Timezone    string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DisplayName == "" {
		body.DisplayName = u.DisplayName
	}
	if body.Email == "" {
		body.Email = u.Email
	}
	if !validEmail(body.Email) {
		httpx.Error(w, http.StatusBadRequest, "invalid email")
		return
	}
	tz := normalizeTimezone(body.Timezone)
	if tz == "UTC" && body.Timezone == "" {
		tz = u.Timezone
	}
	if err := h.Store.UpdateProfile(r.Context(), p.ID, strings.TrimSpace(body.DisplayName), strings.TrimSpace(body.Email), tz); err != nil {
		if errors.Is(err, storage.ErrConflict) || strings.Contains(strings.ToLower(err.Error()), "unique") {
			httpx.Error(w, http.StatusConflict, "email already in use")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err = h.Store.UserByID(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !validPassword(body.Next) {
		httpx.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if _, err := h.Store.Authenticate(r.Context(), p.Username, body.Current); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if err := h.Store.SetPassword(r.Context(), p.ID, body.Next); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	keep := ""
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		keep = c.Value
	}
	_ = h.Store.DeleteSessionsForUserExcept(r.Context(), p.ID, keep)
	h.audit(r, "password.change", "")
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func overviewTask(o storage.CalendarObject) (ds string, allDay bool, loc string, ok bool) {
	cal, err := icsutil.ParseCalendar(o.ICS)
	if err != nil {
		return "", false, "", false
	}
	f := icsutil.TodoFieldsFromCal(cal)
	if strings.EqualFold(f.Status, "COMPLETED") {
		return "", false, "", false
	}
	ds = icsutil.CompactICSTime(f.Due)
	if ds == "" {
		ds = icsutil.CompactICSTime(o.DTEnd)
	}
	if ds == "" {
		ds = icsutil.CompactICSTime(o.DTStart)
	}
	if ds == "" {
		return "", false, "", false
	}
	return ds, !strings.Contains(ds, "T"), icsutil.LocationFromCal(cal), true
}
