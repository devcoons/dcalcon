package caldav

import (
	"net/http"

	"github.com/devcoons/dcalcon/internal/auth"
	icaldav "github.com/devcoons/dcalcon/internal/caldav"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/davpath"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/ratelimit"
	"github.com/devcoons/dcalcon/internal/storage"
)

func New(store *storage.DB, cfg config.Config) http.Handler {
	cal := icaldav.NewHandler(store, cfg.HTTP.PublicURL)
	lim := ratelimit.New(cfg.Auth.MaxAttempts, cfg.Auth.AttemptWindow, cfg.Auth.Lockout)
	basic := auth.Basic(store, cfg.Auth.Realm, lim)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			httpx.Healthz(w, r)
			return
		case "/readyz":
			httpx.Readyz(store)(w, r)
			return
		case "/metrics":
			httpx.ServeMetrics(w, r, cfg.HTTP.MetricsToken)
			return
		case "/.well-known/caldav":
			http.Redirect(w, r, davpath.RootPath, http.StatusMovedPermanently)
			return
		}
		basic(cal).ServeHTTP(w, r)
	})
	return httpx.Wrap(inner, httpx.Options{
		MaxBody:        cfg.HTTP.MaxBodyBytes,
		AllowedOrigins: cfg.AllowedOrigins(),
		PublicHTTPS:    cfg.PublicIsHTTPS(),
		TrustedProxies: cfg.HTTP.TrustedProxies,
	})
}
