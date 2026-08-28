package davext

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/storage"
)

type Handler struct {
	Next  http.Handler
	Store *storage.DB
}

func Wrap(next http.Handler, store *storage.DB) http.Handler {
	return &Handler{Next: next, Store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "MKCALENDAR", "MKCOL":
		if h.handleMkCalendar(w, r) {
			return
		}
	case "ACL":
		if h.handleACL(w, r) {
			return
		}
	case "REPORT":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if dangerousXML(body) {
			http.Error(w, "invalid xml", http.StatusBadRequest)
			return
		}
		if bytes.Contains(bytes.ToLower(body), []byte("sync-collection")) {
			h.handleSync(w, r, body)
			return
		}
		if bytes.Contains(bytes.ToLower(body), []byte("free-busy-query")) {
			h.handleFreeBusy(w, r, body)
			return
		}
	case "POST":
		col, ok := parseCollection(r.URL.Path)
		if ok && strings.EqualFold(col.Slug, "outbox") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			h.handleOutbox(w, r, body)
			return
		}
	case "PROPPATCH":
		if h.handlePropPatch(w, r) {
			return
		}
	}

	if r.Method == "PROPFIND" {
		rec := newRecorder()
		h.Next.ServeHTTP(rec, r)
		out := rec.body.Bytes()
		if rec.code == http.StatusMultiStatus || rec.code == 0 || rec.code == http.StatusOK {
			if inj, err := h.inject(r, out); err == nil && inj != nil {
				out = inj
			}
		}
		copyHeader(w.Header(), rec.hdr)
		addScheduleCapability(w.Header())
		w.Header().Set("Content-Length", strconv.Itoa(len(out)))
		code := rec.code
		if code == 0 {
			code = http.StatusOK
		}
		w.WriteHeader(code)
		_, _ = w.Write(out)
		return
	}

	hw := &headerWriter{ResponseWriter: w}
	h.Next.ServeHTTP(hw, r)
}

type headerWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *headerWriter) WriteHeader(code int) {
	if !w.wrote {
		addScheduleCapability(w.Header())
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *headerWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func addScheduleCapability(h http.Header) {
	dav := h.Get("DAV")
	if dav == "" {
		h.Set("DAV", "1, 3, access-control, calendar-access, addressbook, calendar-auto-schedule")
		return
	}
	if !strings.Contains(dav, "access-control") {
		h.Set("DAV", dav+", access-control")
		dav = h.Get("DAV")
	}
	if !strings.Contains(dav, "calendar-access") {
		h.Set("DAV", dav+", calendar-access")
	}
	if !strings.Contains(h.Get("DAV"), "addressbook") {
		h.Set("DAV", h.Get("DAV")+", addressbook")
	}
	if !strings.Contains(h.Get("DAV"), "calendar-auto-schedule") {
		h.Set("DAV", h.Get("DAV")+", calendar-auto-schedule")
	}
}

type recorder struct {
	hdr  http.Header
	code int
	body bytes.Buffer
}

func newRecorder() *recorder {
	return &recorder{hdr: make(http.Header)}
}

func (r *recorder) Header() http.Header { return r.hdr }

func (r *recorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(b)
}

func (r *recorder) WriteHeader(code int) { r.code = code }

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		dst[k] = vs
	}
}

func dangerousXML(body []byte) bool {
	low := bytes.ToLower(body)
	return bytes.Contains(low, []byte("<!doctype")) || bytes.Contains(low, []byte("<!entity"))
}

type collectionRef struct {
	Kind     string // calendar | addressbook
	Username string
	Slug     string
}

func parseCollection(path string) (collectionRef, bool) {
	path = strings.TrimSuffix(path, "/") + "/"
	switch {
	case strings.HasPrefix(path, davpath.CalendarsRoot):
		rest := strings.TrimPrefix(path, davpath.CalendarsRoot)
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return collectionRef{}, false
		}
		return collectionRef{Kind: "calendar", Username: parts[0], Slug: parts[1]}, true
	case strings.HasPrefix(path, davpath.AddressBooksRoot):
		rest := strings.TrimPrefix(path, davpath.AddressBooksRoot)
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return collectionRef{}, false
		}
		return collectionRef{Kind: "addressbook", Username: parts[0], Slug: parts[1]}, true
	default:
		return collectionRef{}, false
	}
}

func principalOrDeny(r *http.Request) (auth.Principal, bool) {
	return auth.PrincipalFrom(r.Context())
}
