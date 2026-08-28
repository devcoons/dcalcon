package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/ratelimit"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/google/uuid"
)

func (h *Handler) exportCalendar(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarFor(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListCalendarObjects(r.Context(), c.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	name := davpath.ZipSegment(c.Slug) + ".ics"
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	httpx.DispositionAttachment(w, name)
	writeICSBody(w, list)
}

func (h *Handler) importCalendar(w http.ResponseWriter, r *http.Request) {
	c, ok := h.calendarWritable(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "body too large")
			return
		}
		httpx.Error(w, http.StatusBadRequest, "could not read body")
		return
	}
	blocks, err := icsutil.SplitCalendarObjects(string(raw))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(blocks) == 0 {
		httpx.Error(w, http.StatusBadRequest, "no calendar objects found")
		return
	}
	if len(blocks) > limits.MaxImportEvents {
		httpx.Error(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("at most %d events per import", limits.MaxImportEvents))
		return
	}
	created, updated, skipped := 0, 0, 0
	errs := make([]string, 0)
	err = h.Store.WithTx(r.Context(), func(ctx context.Context) error {
		for i, block := range blocks {
			if err := icsutil.CheckICSSize(block); err != nil {
				skipped++
				if len(errs) < 20 {
					errs = append(errs, fmt.Sprintf("item %d: %s", i+1, err.Error()))
				}
				continue
			}
			cal, err := icsutil.ParseCalendar(block)
			if err != nil {
				skipped++
				if len(errs) < 20 {
					errs = append(errs, fmt.Sprintf("item %d: invalid calendar", i+1))
				}
				continue
			}
			uid := icsutil.CalendarUID(cal)
			if uid == "" {
				uid = uuid.NewString()
			}
			comp := icsutil.CalendarComponent(cal)
			href := davpath.ObjectFileHref(uid, ".ics")
			isNew := true
			if existing, err := h.Store.CalendarObjectByUID(ctx, c.ID, uid); err == nil {
				href = existing.Href
				isNew = false
			}
			ds, de := icsutil.CalendarRange(cal)
			if err := h.Store.PutICSWithAttachments(ctx, c.ID, href, uid, comp, block, ds, de, icsutil.CalendarSummary(cal), h.publicURL()); err != nil {
				skipped++
				if len(errs) < 20 {
					errs = append(errs, fmt.Sprintf("item %d: could not save", i+1))
				}
				continue
			}
			if isNew {
				created++
			} else {
				updated++
			}
		}
		return nil
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated, "skipped": skipped, "errors": errs})
}

func (h *Handler) getWebcal(w http.ResponseWriter, r *http.Request) {
	c, ok := h.personalOwnedCalendar(w, r)
	if !ok {
		return
	}
	p := auth.MustPrincipal(r.Context())
	tok, err := h.Store.WebcalForCalendar(r.Context(), p.ID, c.ID)
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"enabled": false, "url": ""})
		return
	}
	out := map[string]any{"enabled": true, "url": ""}
	if validWebcalToken(tok.Token) {
		out["token"] = tok.Token
		out["url"] = h.webcalURL(tok.Token)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) rotateWebcal(w http.ResponseWriter, r *http.Request) {
	c, ok := h.personalOwnedCalendar(w, r)
	if !ok {
		return
	}
	p := auth.MustPrincipal(r.Context())
	tok, err := h.Store.RotateWebcal(r.Context(), p.ID, c.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not rotate webcal")
		return
	}
	h.audit(r, "webcal.rotate", c.Slug)
	httpx.JSON(w, http.StatusOK, map[string]any{"enabled": true, "token": tok.Token, "url": h.webcalURL(tok.Token)})
}

func (h *Handler) deleteWebcal(w http.ResponseWriter, r *http.Request) {
	c, ok := h.personalOwnedCalendar(w, r)
	if !ok {
		return
	}
	p := auth.MustPrincipal(r.Context())
	if err := h.Store.DeleteWebcal(r.Context(), p.ID, c.ID); err != nil && !errors.Is(err, storage.ErrNotFound) {
		httpx.Error(w, http.StatusInternalServerError, "could not disable webcal")
		return
	}
	h.audit(r, "webcal.delete", c.Slug)
	httpx.JSON(w, http.StatusOK, map[string]any{"enabled": false})
}

func (h *Handler) webcalURL(token string) string {
	return h.publicURL() + "/webcal/" + token + ".ics"
}

func writeICSBody(w http.ResponseWriter, list []storage.CalendarObject) {
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(joinObjectICS(list))
}

func joinObjectICS(list []storage.CalendarObject) []byte {
	raws := make([]string, 0, len(list))
	for _, o := range list {
		raws = append(raws, o.ICS)
	}
	return []byte(icsutil.JoinCalendars(raws))
}

func ServeWebcal(store *storage.DB, lim *ratelimit.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ok, retry := lim.Hit("webcal:" + httpx.ClientIP(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		token := strings.TrimPrefix(r.URL.Path, "/webcal/")
		token = strings.TrimSuffix(token, ".ics")
		token = strings.Trim(token, "/")
		if !validWebcalToken(token) {
			http.NotFound(w, r)
			return
		}
		tok, err := store.WebcalByToken(r.Context(), token)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		list, err := store.ListCalendarObjectsByComponent(r.Context(), tok.CalendarID, "VEVENT")
		if err != nil {
			http.Error(w, "calendar", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		writeICSBody(w, list)
	}
}

func validWebcalToken(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}
