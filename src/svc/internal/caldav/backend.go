package caldav

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davcond"
	"github.com/devcoons/dcalcon/internal/davext"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	gocaldav "github.com/emersion/go-webdav/caldav"
)

type Backend struct {
	Store     *storage.DB
	PublicURL string
}

func (b *Backend) principal(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, webdav.NewHTTPError(http.StatusUnauthorized, errors.New("unauthenticated"))
	}
	return p, nil
}

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return "", err
	}
	return davpath.PrincipalPath(p.Username), nil
}

func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return "", err
	}
	return davpath.CalendarHome(p.Username), nil
}

func (b *Backend) toCal(p auth.Principal, c storage.Calendar) *gocaldav.Calendar {
	comps := []string{"VEVENT", "VTODO"}
	if c.Kind == "inbox" || c.Kind == "outbox" {
		comps = []string{"VEVENT", "VTODO", "VFREEBUSY"}
	}
	cal := &gocaldav.Calendar{
		Path:                  davpath.CalendarPath(p.Username, c.Slug),
		Name:                  c.Name,
		Description:           c.Description,
		MaxResourceSize:       8 << 20,
		SupportedComponentSet: comps,
	}
	return cal
}

func (b *Backend) ListCalendars(ctx context.Context) ([]gocaldav.Calendar, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	list, err := b.Store.ListCalendars(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]gocaldav.Calendar, 0, len(list))
	for _, c := range list {
		if c.Shared {
			c.Slug = storage.ShareSlug(c.ID)
			if c.OwnerUsername != "" {
				c.Name = c.OwnerUsername + " — " + c.Name
			}
		}
		out = append(out, *b.toCal(p, c))
	}
	return out, nil
}

func (b *Backend) GetCalendar(ctx context.Context, calPath string) (*gocaldav.Calendar, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	slug := davpath.CalendarSlug(calPath, p.Username)
	if slug == "" {
		return nil, webdav.NewHTTPError(http.StatusNotFound, storage.ErrNotFound)
	}
	c, err := b.Store.CalendarBySlug(ctx, p.ID, slug)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	return b.toCal(p, *c), nil
}

func (b *Backend) CreateCalendar(ctx context.Context, calendar *gocaldav.Calendar) error {
	p, err := b.principal(ctx)
	if err != nil {
		return err
	}
	slug := davpath.CalendarSlug(calendar.Path, p.Username)
	if slug == "" {
		slug = strings.Trim(path.Base(strings.TrimRight(calendar.Path, "/")), "/")
	}
	slug = davpath.Slugify(slug, calendar.Name)
	if !davpath.ValidSlug(slug) {
		return webdav.NewHTTPError(http.StatusBadRequest, errors.New("invalid calendar name"))
	}
	name := calendar.Name
	if name == "" {
		name = slug
	}
	_, err = b.Store.CreateCalendar(ctx, p.ID, slug, name, calendar.Description, "", "personal", false)
	if errors.Is(err, storage.ErrConflict) {
		return webdav.NewHTTPError(http.StatusConflict, err)
	}
	return err
}

func (b *Backend) toObject(p auth.Principal, cal storage.Calendar, o storage.CalendarObject) (*gocaldav.CalendarObject, error) {
	parsed, err := icsutil.ParseCalendar(o.ICS)
	if err != nil {
		return nil, err
	}
	return &gocaldav.CalendarObject{
		Path:          davpath.ObjectPath(davpath.CalendarPath(p.Username, cal.Slug), o.Href),
		ModTime:       o.UpdatedAt,
		ContentLength: int64(len(o.ICS)),
		ETag:          o.ETag,
		Data:          parsed,
	}, nil
}

func (b *Backend) calendarForPath(ctx context.Context, p auth.Principal, objPath string) (*storage.Calendar, string, error) {
	slug := davpath.CalendarSlug(objPath, p.Username)
	if slug == "" {
		return nil, "", webdav.NewHTTPError(http.StatusNotFound, storage.ErrNotFound)
	}
	c, err := b.Store.CalendarBySlug(ctx, p.ID, slug)
	if err != nil {
		return nil, "", webdav.NewHTTPError(http.StatusNotFound, err)
	}
	href := davpath.ObjectHref(objPath, davpath.CalendarPath(p.Username, c.Slug))
	if err := davpath.CheckObjectHref(href); err != nil {
		return nil, "", webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	return c, href, nil
}

func (b *Backend) GetCalendarObject(ctx context.Context, objPath string, req *gocaldav.CalendarCompRequest) (*gocaldav.CalendarObject, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	c, href, err := b.calendarForPath(ctx, p, objPath)
	if err != nil {
		return nil, err
	}
	if href == "" || strings.HasSuffix(objPath, "/") {
		return nil, webdav.NewHTTPError(http.StatusNotFound, storage.ErrNotFound)
	}
	o, err := b.Store.CalendarObjectByHref(ctx, c.ID, href)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	co, err := b.toObject(p, *c, *o)
	if err != nil {
		return nil, err
	}
	applyExpand(co, req)
	return co, nil
}

func (b *Backend) calendarAt(ctx context.Context, calPath string) (auth.Principal, *storage.Calendar, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return auth.Principal{}, nil, err
	}
	slug := davpath.CalendarSlug(calPath, p.Username)
	c, err := b.Store.CalendarBySlug(ctx, p.ID, slug)
	if err != nil {
		return auth.Principal{}, nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	return p, c, nil
}

func (b *Backend) ListCalendarObjects(ctx context.Context, calPath string, req *gocaldav.CalendarCompRequest) ([]gocaldav.CalendarObject, error) {
	p, c, err := b.calendarAt(ctx, calPath)
	if err != nil {
		return nil, err
	}
	list, err := b.Store.ListCalendarObjects(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	out := make([]gocaldav.CalendarObject, 0, len(list))
	for _, o := range list {
		co, err := b.toObject(p, *c, o)
		if err != nil {
			continue
		}
		applyExpand(co, req)
		out = append(out, *co)
	}
	return out, nil
}

func (b *Backend) QueryCalendarObjects(ctx context.Context, calPath string, query *gocaldav.CalendarQuery) ([]gocaldav.CalendarObject, error) {
	p, c, err := b.calendarAt(ctx, calPath)
	if err != nil {
		return nil, err
	}
	want := ""
	var start, end time.Time
	if query != nil {
		want = queryCompName(query.CompFilter)
		start, end = timeRangeOf(query.CompFilter)
	}
	var list []storage.CalendarObject
	if want != "" {
		list, err = b.Store.ListCalendarObjectsByComponent(ctx, c.ID, want)
	} else {
		list, err = b.Store.ListCalendarObjects(ctx, c.ID)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]gocaldav.CalendarObject, 0, len(list))
	for _, o := range list {
		co, err := b.toObject(p, *c, o)
		if err != nil {
			continue
		}
		if (!start.IsZero() || !end.IsZero()) && co.Data != nil && !icsutil.OverlapsTimeRange(co.Data, start, end) {
			continue
		}
		if query != nil {
			applyExpand(co, &query.CompRequest)
		}
		filtered = append(filtered, *co)
	}
	return filtered, nil
}

func queryCompName(cf gocaldav.CompFilter) string {
	n := strings.ToUpper(strings.TrimSpace(cf.Name))
	switch n {
	case "VEVENT", "VTODO", "VJOURNAL", "VFREEBUSY":
		return n
	}
	for _, c := range cf.Comps {
		if name := queryCompName(c); name != "" {
			return name
		}
	}
	return ""
}

func timeRangeOf(cf gocaldav.CompFilter) (time.Time, time.Time) {
	if !cf.Start.IsZero() || !cf.End.IsZero() {
		return cf.Start, cf.End
	}
	for _, p := range cf.Props {
		if !p.Start.IsZero() || !p.End.IsZero() {
			return p.Start, p.End
		}
	}
	for _, c := range cf.Comps {
		if s, e := timeRangeOf(c); !s.IsZero() || !e.IsZero() {
			return s, e
		}
	}
	return time.Time{}, time.Time{}
}

func (b *Backend) PutCalendarObject(ctx context.Context, objPath string, calendar *ical.Calendar, opts *gocaldav.PutCalendarObjectOptions) (*gocaldav.CalendarObject, error) {
	p, err := b.principal(ctx)
	if err != nil {
		return nil, err
	}
	c, href, err := b.calendarForPath(ctx, p, objPath)
	if err != nil {
		return nil, err
	}
	if err := calendarWriteError(*c, false); err != nil {
		return nil, err
	}
	if href == "" {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, errors.New("missing object href"))
	}
	existing, err := b.Store.CalendarObjectByHref(ctx, c.ID, href)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	prevICS := ""
	if existing != nil {
		prevICS = existing.ICS
	}
	etag := ""
	if existing != nil {
		etag = existing.ETag
	}
	var ifMatch, ifNone webdav.ConditionalMatch
	if opts != nil {
		ifMatch, ifNone = opts.IfMatch, opts.IfNoneMatch
	}
	if err := davcond.Check(etag, existing != nil, ifMatch, ifNone); err != nil {
		return nil, err
	}
	if c.Kind != "inbox" && c.Kind != "outbox" {
		if _, _, err := gocaldav.ValidateCalendarObject(calendar); err != nil {
			return nil, webdav.NewHTTPError(http.StatusForbidden, err)
		}
	}
	comp := icsutil.CalendarComponent(calendar)
	if !componentAllowed(c.Kind, comp) {
		return nil, webdav.NewHTTPError(http.StatusForbidden, errors.New("component not allowed on this collection"))
	}
	raw, err := icsutil.EncodeCalendar(calendar)
	if err != nil {
		return nil, err
	}
	if err := icsutil.CheckICSSize(raw); err != nil {
		return nil, webdav.NewHTTPError(http.StatusRequestEntityTooLarge, err)
	}
	uid := icsutil.CalendarUID(calendar)
	if uid == "" {
		uid = strings.TrimSuffix(href, path.Ext(href))
	}
	ds, de := icsutil.CalendarRange(calendar)
	if err := b.Store.PutICSWithAttachments(ctx, c.ID, href, uid, icsutil.CalendarComponent(calendar), raw, ds, de, icsutil.CalendarSummary(calendar), b.PublicURL); err != nil {
		if st := storage.AttachLimitStatus(err); st == http.StatusRequestEntityTooLarge {
			return nil, webdav.NewHTTPError(http.StatusRequestEntityTooLarge, err)
		}
		if st := storage.AttachLimitStatus(err); st != 0 {
			return nil, webdav.NewHTTPError(st, err)
		}
		return nil, err
	}
	o, err := b.Store.CalendarObjectByHref(ctx, c.ID, href)
	if err != nil {
		return nil, err
	}
	if u, err := b.Store.UserByID(ctx, p.ID); err != nil {
		slog.Error("schedule deliver: organizer", "err", err)
	} else if err := schedule.DeliverFromPut(ctx, b.Store, u, c, o, prevICS); err != nil {
		slog.Error("schedule deliver", "err", err, "uid", o.UID)
		metrics.IncScheduleError()
	}
	return b.toObject(p, *c, *o)
}

func (b *Backend) DeleteCalendarObject(ctx context.Context, objPath string) error {
	p, err := b.principal(ctx)
	if err != nil {
		return err
	}
	c, href, err := b.calendarForPath(ctx, p, objPath)
	if err != nil {
		return err
	}
	if err := calendarWriteError(*c, true); err != nil {
		return err
	}
	existing, err := b.Store.CalendarObjectByHref(ctx, c.ID, href)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	if existing != nil {
		if u, err := b.Store.UserByID(ctx, p.ID); err == nil {
			if err := schedule.CancelFromDelete(ctx, b.Store, u, c, existing); err != nil {
				slog.Error("schedule cancel", "err", err, "uid", existing.UID)
				metrics.IncScheduleError()
			}
		}
	}
	if err := b.Store.DeleteCalendarObject(ctx, c.ID, href); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return webdav.NewHTTPError(http.StatusNotFound, err)
		}
		return err
	}
	return nil
}

func calendarWriteError(c storage.Calendar, isDelete bool) error {
	switch c.Kind {
	case "important_dates":
		return webdav.NewHTTPError(http.StatusForbidden, errors.New("calendar is read-only"))
	case "inbox":
		if isDelete {
			return nil
		}
		return webdav.NewHTTPError(http.StatusForbidden, errors.New("schedule inbox is not client-writable"))
	}
	if c.ReadOnly && c.Kind != "outbox" {
		return webdav.NewHTTPError(http.StatusForbidden, errors.New("calendar is read-only"))
	}
	return nil
}

func componentAllowed(kind, comp string) bool {
	comp = strings.ToUpper(strings.TrimSpace(comp))
	switch kind {
	case "inbox", "outbox":
		return comp == "VEVENT" || comp == "VTODO" || comp == "VFREEBUSY"
	default:
		return comp == "VEVENT" || comp == "VTODO"
	}
}

func applyExpand(co *gocaldav.CalendarObject, req *gocaldav.CalendarCompRequest) {
	if co == nil || co.Data == nil || req == nil || req.Expand == nil {
		return
	}
	co.Data = icsutil.ExpandCalendar(co.Data, req.Expand.Start, req.Expand.End)
}

func NewHandler(store *storage.DB, publicURL string) http.Handler {
	inner := &gocaldav.Handler{Backend: &Backend{Store: store, PublicURL: strings.TrimRight(publicURL, "/")}, Prefix: "/dav"}
	next := davext.Wrap(inner, store)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dav/attachments/") {
			ServeAttachment(store, w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
