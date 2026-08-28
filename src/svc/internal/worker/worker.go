package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/storage"
)

type Worker struct {
	Store      *storage.DB
	Cfg        config.Config
	Log        *slog.Logger
	lastBackup time.Time
}

func (w *Worker) Run(ctx context.Context) {
	if w.Log == nil {
		w.Log = slog.Default()
	}
	t := time.NewTicker(w.Cfg.Worker.Interval)
	defer t.Stop()
	w.Log.Info("worker started", "interval", w.Cfg.Worker.Interval)
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			w.Log.Info("worker stopped")
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	st, err := w.Store.PurgeExpired(pctx)
	cancel()
	if err != nil {
		w.Log.Error("purge", "err", err)
	} else if st.Sessions+st.OAuth+st.Recovery > 0 {
		w.Log.Info("purged expired rows", "sessions", st.Sessions, "oauth", st.OAuth, "recovery", st.Recovery)
	}
	sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	err = w.syncImportantDates(sctx)
	cancel()
	if err != nil {
		w.Log.Error("important dates", "err", err)
	}
	bctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	w.maybeBackup(bctx)
	cancel()
}

func (w *Worker) maybeBackup(ctx context.Context) {
	if strings.TrimSpace(w.Cfg.Backup.Dir) == "" {
		return
	}
	if !w.lastBackup.IsZero() && time.Since(w.lastBackup) < w.Cfg.Backup.Interval {
		return
	}
	name := time.Now().UTC().Format("dcalcon-20060102-150405.db")
	dest := filepath.Join(w.Cfg.Backup.Dir, name)
	if err := w.Store.Backup(ctx, dest); err != nil {
		metrics.IncBackupError()
		w.Log.Error("backup", "err", err, "path", dest)
		return
	}
	metrics.IncBackupOK()
	w.lastBackup = time.Now()
	if err := storage.PruneBackups(w.Cfg.Backup.Dir, w.Cfg.Backup.Keep); err != nil {
		w.Log.Error("prune backups", "err", err)
	}
	if err := storage.RunBackupHook(ctx, w.Cfg.Backup.Hook, dest); err != nil {
		w.Log.Error("backup hook", "err", err, "path", dest)
	}
	w.Log.Info("backup written", "path", dest)
}

func (w *Worker) syncImportantDates(ctx context.Context) error {
	users, err := w.Store.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		s, err := w.Store.GetImportantDates(ctx, u.ID)
		if err != nil {
			return err
		}
		if !s.Enabled {
			cal, err := w.Store.CalendarBySlug(ctx, u.ID, "important-dates")
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if err := w.Store.DeleteCalendarObjectsByPrefix(ctx, cal.ID, "id-"); err != nil {
				return err
			}
			continue
		}
		cal, err := w.Store.EnsureCalendar(ctx, u.ID, "important-dates", "Important Dates", "important_dates", true)
		if err != nil {
			return err
		}
		contacts, err := w.Store.ContactsWithDates(ctx, u.ID)
		if err != nil {
			return err
		}
		if err := w.applyImportantDates(ctx, cal.ID, contacts, s); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) applyImportantDates(ctx context.Context, calID int64, contacts []storage.AddressObject, s *storage.ImportantDatesSettings) error {
	existing, err := w.Store.ListCalendarObjectRefs(ctx, calID, "id-")
	if err != nil {
		return err
	}
	have := make(map[string]string, len(existing))
	for _, r := range existing {
		have[r.Href] = r.ETag
	}
	want := make(map[string]struct{})
	for _, c := range contacts {
		if s.IncludeBirthdays && c.BDAY != "" {
			if err := w.upsertDay(ctx, calID, c, "bday", "Birthday", c.BDAY, s.AlarmOffsets, have, want); err != nil {
				return err
			}
		}
		if s.IncludeAnniversaries && c.Anniversary != "" {
			if err := w.upsertDay(ctx, calID, c, "ann", "Anniversary", c.Anniversary, s.AlarmOffsets, have, want); err != nil {
				return err
			}
		}
	}
	for href := range have {
		if _, ok := want[href]; ok {
			continue
		}
		if err := w.Store.DeleteCalendarObject(ctx, calID, href); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (w *Worker) upsertDay(ctx context.Context, calID int64, c storage.AddressObject, kind, label, date string, alarms []string, have map[string]string, want map[string]struct{}) error {
	md := normalizeMonthDay(date)
	if md == "" {
		return nil
	}
	name := c.FN
	if name == "" {
		name = c.UID
	}
	uid := fmt.Sprintf("id-%s-%s@dcalcon", kind, c.UID)
	href := fmt.Sprintf("id-%s-%s.ics", kind, sanitize(c.UID))
	year := time.Now().UTC().Year()
	dtstart := fmt.Sprintf("%04d%s", year, md)
	summary := name + " — " + label
	ics := importantICS(uid, summary, dtstart, alarms)
	etag := icsutil.ETag(ics)
	want[href] = struct{}{}
	if have[href] == etag {
		return nil
	}
	return w.Store.UpsertCalendarObject(ctx, calID, href, uid, etag, "VEVENT", ics, dtstart, "", summary)
}

func importantICS(uid, summary, dtstart string, alarms []string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//dCalCon//Important Dates//EN\r\nCALSCALE:GREGORIAN\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	fmt.Fprintf(&b, "UID:%s\r\n", uid)
	fmt.Fprintf(&b, "DTSTAMP:%sT000000Z\r\n", dtstart)
	fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\n", dtstart)
	b.WriteString("RRULE:FREQ=YEARLY\r\n")
	fmt.Fprintf(&b, "SUMMARY:%s\r\n", summary)
	b.WriteString("TRANSP:TRANSPARENT\r\nX-DCALCON-MANAGED:1\r\n")
	for _, a := range alarms {
		if a == "" || a == "PT0S" {
			continue
		}
		b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\n")
		fmt.Fprintf(&b, "TRIGGER:%s\r\n", a)
		fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", summary)
		b.WriteString("END:VALARM\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func normalizeMonthDay(raw string) string {
	s := strings.ReplaceAll(strings.TrimSpace(raw), "-", "")
	s = strings.TrimPrefix(s, "--")
	switch {
	case len(s) == 8:
		return s[4:]
	case len(s) == 4:
		return s
	default:
		return ""
	}
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	return s
}

func Handler(store *storage.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", httpx.Healthz)
	mux.HandleFunc("/readyz", httpx.Readyz(store))
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) { metrics.Write(w) })
	return mux
}
