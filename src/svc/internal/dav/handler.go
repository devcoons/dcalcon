package dav

import (
	"net/http"
	"strings"

	"github.com/devcoons/dcalcon/internal/auth"
	icaldav "github.com/devcoons/dcalcon/internal/caldav"
	icarddav "github.com/devcoons/dcalcon/internal/carddav"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/ratelimit"
	"github.com/devcoons/dcalcon/internal/storage"
)

type Handler struct {
	Store   *storage.DB
	CalDAV  http.Handler
	CardDAV http.Handler
	Auth    func(http.Handler) http.Handler
}

func New(store *storage.DB, realm string, lim *ratelimit.Limiter, publicURL string) *Handler {
	return &Handler{
		Store:   store,
		CalDAV:  icaldav.NewHandler(store, publicURL),
		CardDAV: icarddav.NewHandler(store),
		Auth:    auth.Basic(store, realm, lim),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/.well-known/caldav" || strings.HasPrefix(p, "/.well-known/caldav"):
		http.Redirect(w, r, davpath.RootPath, http.StatusMovedPermanently)
		return
	case p == "/.well-known/carddav" || strings.HasPrefix(p, "/.well-known/carddav"):
		http.Redirect(w, r, davpath.RootPath, http.StatusMovedPermanently)
		return
	}

	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case p == "/dav" || p == "/dav/" || strings.HasPrefix(p, davpath.PrincipalsRoot):
			h.servePrincipal(w, r)
		case strings.HasPrefix(p, "/dav/attachments/"):
			icaldav.ServeAttachment(h.Store, w, r)
		case strings.HasPrefix(p, davpath.CalendarsRoot):
			h.CalDAV.ServeHTTP(w, r)
		case strings.HasPrefix(p, davpath.AddressBooksRoot):
			h.CardDAV.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	h.Auth(protected).ServeHTTP(w, r)
}
