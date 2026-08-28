package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/mail"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/ratelimit"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

type Handler struct {
	Store  *storage.DB
	Cfg    config.Config
	Mailer mail.Sender
	HTTP   *http.Client
	Limit  *ratelimit.Limiter
}

func New(store *storage.DB, cfg config.Config) http.Handler {
	return NewHandler(store, cfg).Routes()
}

func NewHandler(store *storage.DB, cfg config.Config) *Handler {
	return &Handler{
		Store:  store,
		Cfg:    cfg,
		Mailer: mail.New(cfg),
		Limit:  ratelimit.New(cfg.Auth.MaxAttempts, cfg.Auth.AttemptWindow, cfg.Auth.Lockout),
	}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   h.Cfg.AllowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Content-Disposition", "ETag", "X-Request-ID"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", httpx.Healthz)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusNotFound, "not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	})
	r.Post("/api/v1/auth/login", h.login)
	r.Post("/api/v1/auth/logout", h.logout)
	r.Post("/api/v1/auth/recover", h.recover)
	r.Post("/api/v1/auth/reset", h.resetPassword)
	r.Post("/api/v1/auth/reset-totp", h.resetWithTOTP)
	r.Get("/api/v1/oauth/{provider}/callback", h.oauthCallback)

	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler { return auth.SessionOrBearer(h.Store)(next) })
		r.Get("/api/v1/me", h.me)
		r.Get("/api/v1/me/app-passwords", h.listAppPasswords)
		r.Post("/api/v1/me/app-passwords", h.createAppPassword)
		r.Delete("/api/v1/me/app-passwords/{id}", h.deleteAppPassword)
		r.Post("/api/v1/me/sessions/revoke", h.revokeSessions)
		r.Get("/api/v1/me/export", h.exportTakeout)
		r.Get("/api/v1/me/backup", h.exportBackupGET)
		r.Post("/api/v1/me/backup/export", h.exportBackupPOST)
		r.Post("/api/v1/me/backup", h.importBackup)
		r.Patch("/api/v1/me", h.patchMe)
		r.Post("/api/v1/me/password", h.changePassword)
		r.Post("/api/v1/me/totp/setup", h.totpSetup)
		r.Post("/api/v1/me/totp/enable", h.totpEnable)
		r.Post("/api/v1/me/totp/disable", h.totpDisable)
		r.Delete("/api/v1/me/totp/setup", h.totpCancel)
		r.Get("/api/v1/overview", h.overview)
		r.Get("/api/v1/setup", h.setup)
		r.Get("/api/v1/directory", h.directory)
		r.Get("/api/v1/freebusy", h.freeBusy)
		r.Get("/api/v1/tasks", h.listTasks)
		r.Get("/api/v1/calendars", h.listCalendars)
		r.Post("/api/v1/calendars", h.createCalendar)
		r.Patch("/api/v1/calendars/{id}", h.patchCalendar)
		r.Delete("/api/v1/calendars/{id}", h.deleteCalendar)
		r.Get("/api/v1/calendars/{id}/shares", h.listShares)
		r.Post("/api/v1/calendars/{id}/shares", h.createShare)
		r.Delete("/api/v1/calendars/{id}/shares/{userId}", h.deleteShare)
		r.Get("/api/v1/calendars/{id}/attachments/{attId}", h.downloadAttachment)
		r.Get("/api/v1/calendars/{id}/events", h.listEvents)
		r.Post("/api/v1/calendars/{id}/events", h.createEvent)
		r.Get("/api/v1/calendars/{id}/events/{href}/attachments", h.listEventAttachments)
		r.Post("/api/v1/calendars/{id}/events/{href}/attachments", h.uploadEventAttachments)
		r.Delete("/api/v1/calendars/{id}/events/{href}/attachments/{attId}", h.deleteEventAttachment)
		r.Get("/api/v1/calendars/{id}/events/{href}", h.getEvent)
		r.Put("/api/v1/calendars/{id}/events/{href}", h.updateEvent)
		r.Delete("/api/v1/calendars/{id}/events/{href}", h.deleteEvent)
		r.Get("/api/v1/calendars/{id}/export", h.exportCalendar)
		r.Post("/api/v1/calendars/{id}/import", h.importCalendar)
		r.Get("/api/v1/calendars/{id}/webcal", h.getWebcal)
		r.Post("/api/v1/calendars/{id}/webcal", h.rotateWebcal)
		r.Delete("/api/v1/calendars/{id}/webcal", h.deleteWebcal)
		r.Get("/api/v1/calendars/{id}/tasks", h.listCalendarTasks)
		r.Post("/api/v1/calendars/{id}/tasks", h.createTask)
		r.Get("/api/v1/calendars/{id}/tasks/{href}", h.getTask)
		r.Get("/api/v1/calendars/{id}/tasks/{href}/attachments", h.listTaskAttachments)
		r.Post("/api/v1/calendars/{id}/tasks/{href}/attachments", h.uploadTaskAttachments)
		r.Delete("/api/v1/calendars/{id}/tasks/{href}/attachments/{attId}", h.deleteTaskAttachment)
		r.Put("/api/v1/calendars/{id}/tasks/{href}", h.updateTask)
		r.Delete("/api/v1/calendars/{id}/tasks/{href}", h.deleteTask)
		r.Get("/api/v1/addressbooks", h.listAddressBooks)
		r.Get("/api/v1/addressbooks/{id}/contacts", h.listContacts)
		r.Post("/api/v1/addressbooks/{id}/contacts", h.createContact)
		r.Get("/api/v1/addressbooks/{id}/contacts/export", h.exportContacts)
		r.Post("/api/v1/addressbooks/{id}/contacts/import", h.importContacts)
		r.Get("/api/v1/addressbooks/{id}/contacts/{href}/vcard", h.exportContact)
		r.Get("/api/v1/addressbooks/{id}/contacts/{href}", h.getContact)
		r.Put("/api/v1/addressbooks/{id}/contacts/{href}", h.updateContact)
		r.Delete("/api/v1/addressbooks/{id}/contacts/{href}", h.deleteContact)
		r.Get("/api/v1/invitations", h.listInvitations)
		r.Post("/api/v1/invitations/{id}/accept", h.respondInvitation(true))
		r.Post("/api/v1/invitations/{id}/decline", h.respondInvitation(false))
		r.Post("/api/v1/events/{calendarId}/invite", h.invite)
		r.Get("/api/v1/settings/important-dates", h.getImportantDates)
		r.Put("/api/v1/settings/important-dates", h.putImportantDates)
		r.Get("/api/v1/accounts", h.listAccounts)
		r.Post("/api/v1/accounts", h.connectAccount)
		r.Delete("/api/v1/accounts/{id}", h.deleteAccount)
		r.Post("/api/v1/accounts/{id}/test", h.testAccount)
		r.Get("/api/v1/mail", h.mailStatus)

		r.Group(func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler { return auth.RequireAdmin(next) })
			r.Get("/api/v1/admin/users", h.adminListUsers)
			r.Post("/api/v1/admin/users", h.adminCreateUser)
			r.Patch("/api/v1/admin/users/{id}", h.adminPatchUser)
			r.Post("/api/v1/admin/users/{id}/password", h.adminResetPassword)
			r.Post("/api/v1/admin/users/{id}/recovery", h.adminSendRecovery)
			r.Post("/api/v1/admin/users/{id}/totp/disable", h.adminDisableTOTP)
			r.Get("/api/v1/admin/recovery-outbox", h.adminRecoveryOutbox)
			r.Get("/api/v1/admin/audit", h.adminAudit)
		})
	})
	return r
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	key := "login:" + httpx.ClientIP(r) + ":" + strings.ToLower(strings.TrimSpace(body.Username))
	if ok, retry := h.Limit.Allow(key); !ok {
		metrics.IncAuthLockout()
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	u, err := h.authenticateLogin(r, body.Username, body.Password, body.TOTP)
	if errors.Is(err, errTOTPRequired) {
		httpx.Error(w, http.StatusUnauthorized, "authenticator code required")
		return
	}
	if err != nil {
		locked, retry := h.Limit.Fail(key)
		metrics.IncAuthFail()
		if locked {
			metrics.IncAuthLockout()
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			httpx.Error(w, http.StatusTooManyRequests, "too many attempts, try again later")
			return
		}
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	h.Limit.Success(key)
	sid, err := auth.NewSessionID()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "session")
		return
	}
	if err := h.Store.CreateSession(r.Context(), sid, u.ID, h.Cfg.Auth.SessionTTL); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "session")
		return
	}
	http.SetCookie(w, h.sessionCookie(sid, time.Now().Add(h.Cfg.Auth.SessionTTL)))
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) sessionCookie(value string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     auth.SessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.Cfg.Auth.SessionSecure,
	}
	if value == "" {
		c.MaxAge = -1
		c.Expires = time.Unix(0, 0)
	} else {
		c.Expires = expires
	}
	return c
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = h.Store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, h.sessionCookie("", time.Time{}))
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	u, err := h.Store.UserByID(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user")
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) listCalendars(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	list, err := h.Store.ListCalendars(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.Calendar{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) createCalendar(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var body struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	body.Slug = davpath.Slugify(body.Slug, body.Name)
	c, err := h.Store.CreateCalendar(r.Context(), p.ID, body.Slug, body.Name, body.Description, body.Color, "personal", false)
	if errors.Is(err, storage.ErrConflict) {
		httpx.Error(w, http.StatusConflict, "calendar exists")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

func writeDenied(w http.ResponseWriter, c *storage.Calendar) bool {
	if c.CanWrite() {
		return false
	}
	httpx.Error(w, http.StatusForbidden, "read-only calendar")
	return true
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListCalendarObjectsByComponent(r.Context(), c.ID, "VEVENT")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	atts := h.attachmentMap(r.Context(), c.ID)
	out := make([]eventDTO, 0, len(list))
	for _, o := range list {
		out = append(out, toEventDTO(o, atts[o.Href]))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	var body eventWrite
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := body.prepare(); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	if body.UID == "" {
		body.UID = uuid.NewString()
	}
	href := body.UID + ".ics"
	if err := davpath.CheckObjectHref(href); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid uid")
		return
	}
	ics := icsutil.NewEventICS(body.UID, body.Summary, body.DTStart, body.DTEnd, body.Description, body.Location)
	ics = applyEventExtras(ics, body)
	etag := icsutil.ETag(ics)
	if err := h.Store.UpsertCalendarObject(r.Context(), c.ID, href, body.UID, etag, "VEVENT", ics, body.DTStart, body.DTEnd, body.Summary); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"href": href, "uid": body.UID, "etag": etag}
	if obj, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href); err == nil {
		if inv := h.maybeInvite(r, p, c, obj, body.Invite, body.InviteEmails); inv != nil {
			resp["invite"] = inv
		}
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "event")
	if !ok {
		return
	}
	o, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || isTodo(*o) {
		httpx.Error(w, http.StatusNotFound, "event")
		return
	}
	httpx.JSON(w, http.StatusOK, toEventDTO(*o, h.attachmentsFor(r.Context(), c.ID, o.Href)))
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "event")
	if !ok {
		return
	}
	existing, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || isTodo(*existing) {
		httpx.Error(w, http.StatusNotFound, "event")
		return
	}
	var body eventWrite
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := body.prepare(); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	ics, err := icsutil.UpdateEventICS(existing.ICS, body.Summary, body.DTStart, body.DTEnd, body.Description, body.Location)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "could not update event")
		return
	}
	ics = applyEventExtras(ics, body)
	etag := icsutil.ETag(ics)
	if err := h.Store.UpsertCalendarObject(r.Context(), c.ID, href, existing.UID, etag, existing.Component, ics, body.DTStart, body.DTEnd, body.Summary); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	o, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	inv := h.maybeInvite(r, p, c, o, body.Invite, body.InviteEmails)
	if inv != nil {
		o, _ = h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	}
	httpx.JSON(w, http.StatusOK, eventWithInvite{eventDTO: toEventDTO(*o, h.attachmentsFor(r.Context(), c.ID, href)), Invite: inv})
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	href, ok := objectHref(w, r, "event")
	if !ok {
		return
	}
	existing, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, href)
	if err != nil || isTodo(*existing) {
		httpx.Error(w, http.StatusNotFound, "event")
		return
	}
	if u, err := h.Store.UserByID(r.Context(), p.ID); err == nil {
		if err := schedule.CancelFromDelete(r.Context(), h.Store, u, c, existing); err != nil {
			slog.Error("schedule cancel", "err", err, "uid", existing.UID)
			metrics.IncScheduleError()
		}
	}
	if err := h.Store.DeleteCalendarObject(r.Context(), c.ID, href); err != nil {
		httpx.Error(w, http.StatusNotFound, "event")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listInvitations(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	list, err := h.Store.ListInbox(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]invitationDTO, 0, len(list))
	for _, it := range list {
		out = append(out, toInvitationDTO(it))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) respondInvitation(accept bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := auth.MustPrincipal(r.Context())
		id, ok := pathID(r, "id")
		if !ok {
			httpx.Error(w, http.StatusNotFound, "invitation")
			return
		}
		item, err := h.Store.GetScheduleItem(r.Context(), p.ID, id)
		if err != nil {
			httpx.Error(w, http.StatusNotFound, "invitation")
			return
		}
		if item.Status != "pending" {
			httpx.Error(w, http.StatusBadRequest, "invitation already processed")
			return
		}
		u, err := h.Store.UserByID(r.Context(), p.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if accept {
			var body struct {
				CalendarID int64 `json:"calendar_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if err := schedule.Accept(r.Context(), h.Store, u, item, body.CalendarID); err != nil {
				httpx.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			httpx.JSON(w, http.StatusOK, map[string]string{"status": "accepted"})
			return
		}
		if err := schedule.Decline(r.Context(), h.Store, u, item); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "declined"})
	}
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	c, ok := h.calendarByParam(w, r, "calendarId")
	if !ok {
		return
	}
	if writeDenied(w, c) {
		return
	}
	var body struct {
		Href      string   `json:"href"`
		Usernames []string `json:"usernames"`
		Emails    []string `json:"emails"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := davpath.CheckObjectHref(body.Href); err != nil {
		httpx.Error(w, http.StatusNotFound, "event")
		return
	}
	obj, err := h.Store.CalendarObjectByHref(r.Context(), c.ID, body.Href)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "event")
		return
	}
	sent, err := h.sendInvites(r, p, c, obj, body.Usernames, body.Emails)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if sent.MailError != "" && sent.Email == 0 && sent.Local == 0 {
		httpx.Error(w, http.StatusBadRequest, sent.MailError)
		return
	}
	status := "sent"
	if sent.MailError != "" {
		status = "partial"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": status, "sent": sent.Local + sent.Email, "local": sent.Local, "email": sent.Email, "missing": sent.Missing, "mail_error": sent.MailError})
}

func (h *Handler) getImportantDates(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	s, err := h.Store.GetImportantDates(r.Context(), p.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) putImportantDates(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	var s storage.ImportantDatesSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.Store.SaveImportantDates(r.Context(), p.ID, s); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) adminListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListUsers(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []storage.User{}
	}
	httpx.JSON(w, http.StatusOK, list)
}
