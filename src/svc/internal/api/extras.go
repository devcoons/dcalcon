package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/schedule"
)

func (h *Handler) audit(r *http.Request, action, detail string) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		h.Store.Audit(r.Context(), 0, "", action, detail)
		return
	}
	h.Store.Audit(r.Context(), p.ID, p.Username, action, detail)
}

func (h *Handler) revokeSessions(w http.ResponseWriter, r *http.Request) {
	p := auth.MustPrincipal(r.Context())
	keep := ""
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		keep = c.Value
	}
	if err := h.Store.DeleteSessionsForUserExcept(r.Context(), p.ID, keep); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "sessions.revoke", "other sessions")
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) adminAudit(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListAudit(r.Context(), 200)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

func (h *Handler) freeBusy(w http.ResponseWriter, r *http.Request) {
	auth.MustPrincipal(r.Context())
	users := strings.Split(r.URL.Query().Get("users"), ",")
	start, _ := icsutil.ParseICSTime(r.URL.Query().Get("start"))
	end, _ := icsutil.ParseICSTime(r.URL.Query().Get("end"))
	if start.IsZero() {
		start = time.Now().UTC()
	}
	if end.IsZero() || !end.After(start) {
		end = start.Add(24 * time.Hour)
	}
	out := map[string][]icsutil.BusyPeriod{}
	n := 0
	for _, name := range users {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		n++
		if n > 20 {
			httpx.Error(w, http.StatusBadRequest, "too many users")
			return
		}
		u, err := schedule.FindUser(r.Context(), h.Store, name)
		if err != nil {
			out[name] = []icsutil.BusyPeriod{}
			continue
		}
		periods, err := schedule.BusyForUser(r.Context(), h.Store, u.ID, start, end)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if periods == nil {
			periods = []icsutil.BusyPeriod{}
		}
		out[u.Username] = periods
	}
	httpx.JSON(w, http.StatusOK, out)
}
