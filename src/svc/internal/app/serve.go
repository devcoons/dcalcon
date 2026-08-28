package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/api"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/dav"
	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/ratelimit"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/devcoons/dcalcon/internal/version"
	"github.com/devcoons/dcalcon/internal/worker"
)

func CombinedHandler(store *storage.DB, cfg config.Config) http.Handler {
	apiH := api.NewHandler(store, cfg)
	apiRoutes := apiH.Routes()
	davH := dav.New(store, cfg.Auth.Realm, apiH.Limit, cfg.HTTP.PublicURL)
	webcalLim := ratelimit.New(120, time.Hour, time.Minute)
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/healthz":
			httpx.Healthz(w, r)
		case p == "/readyz":
			httpx.Readyz(store)(w, r)
		case p == "/metrics":
			httpx.ServeMetrics(w, r, cfg.HTTP.MetricsToken)
		case p == "/version":
			httpx.JSON(w, http.StatusOK, map[string]string{"name": version.Name, "version": version.Version})
		case strings.HasPrefix(p, "/webcal/"):
			api.ServeWebcal(store, webcalLim)(w, r)
		case strings.HasPrefix(p, "/api/"), p == "/api":
			apiRoutes.ServeHTTP(w, r)
		default:
			davH.ServeHTTP(w, r)
		}
	})
	return wrapHTTP(cfg, mux)
}

func wrapHTTP(cfg config.Config, next http.Handler) http.Handler {
	return httpx.Wrap(next, httpx.Options{
		MaxBody:        cfg.HTTP.MaxBodyBytes,
		AllowedOrigins: cfg.AllowedOrigins(),
		PublicHTTPS:    cfg.PublicIsHTTPS(),
		TrustedProxies: cfg.HTTP.TrustedProxies,
	})
}

func Listen(ctx context.Context, addr string, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func StartWorker(ctx context.Context, store *storage.DB, cfg config.Config, log *slog.Logger) {
	w := &worker.Worker{Store: store, Cfg: cfg, Log: log.With("component", "worker")}
	go w.Run(ctx)
}
