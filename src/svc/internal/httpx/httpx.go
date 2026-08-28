package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/limits"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/storage"
)

type ctxKey int

const requestIDKey ctxKey = 1

const sessionCookie = "dcalcon_session"

type Options struct {
	MaxBody        int64
	AllowedOrigins []string
	Log            *slog.Logger
	PublicHTTPS    bool
	TrustedProxies []string
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]any{"error": msg})
}

func Healthz(w http.ResponseWriter, _ *http.Request) {
	JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func Readyz(store *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil || store.SQL == nil {
			Error(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			Error(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		var n int
		if err := store.SQL.QueryRowContext(ctx, `SELECT 1`).Scan(&n); err != nil {
			Error(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func RequestIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(requestIDKey).(string)
	return s
}

func Wrap(next http.Handler, opts Options) http.Handler {
	maxBody := opts.MaxBody
	if maxBody <= 0 {
		maxBody = limits.MaxHTTPBody
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	trusted, err := ParseTrustedProxies(opts.TrustedProxies)
	if err != nil {
		trusted, _ = ParseTrustedProxies(nil)
	}
	h := OriginGuard(opts.AllowedOrigins)(next)
	h = MaxBody(maxBody)(h)
	h = SecurityHeaders(opts.PublicHTTPS)(h)
	h = RequestID(h)
	h = AccessLog(log)(h)
	return withClientIP(h, trusted)
}

func SecurityHeaders(publicHTTPS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
			w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
			if publicHTTPS || r.TLS != nil {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func validRequestID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func MaxBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				limit := n
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/me/backup" {
					limit = limits.MaxBackupZip
				}
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func OriginGuard(allowed []string) func(http.Handler) http.Handler {
	allow := map[string]bool{}
	for _, o := range allowed {
		allow[strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if hasSessionCookie(r) && strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
				Error(w, http.StatusForbidden, "origin not allowed")
				return
			}
			origin := strings.TrimRight(strings.ToLower(strings.TrimSpace(r.Header.Get("Origin"))), "/")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if allow[origin] {
				next.ServeHTTP(w, r)
				return
			}
			Error(w, http.StatusForbidden, "origin not allowed")
		})
	}
}

func hasSessionCookie(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && c != nil && c.Value != ""
}

func AccessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(rw, r)
			if strings.HasPrefix(r.URL.Path, "/dav") || strings.HasPrefix(r.URL.Path, "/.well-known/") {
				metrics.IncDAV(r.Method)
			}
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.code,
				"ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()),
				"ip", ClientIP(r),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func ServeMetrics(w http.ResponseWriter, r *http.Request, token string) {
	if !AllowMetrics(r, token) {
		http.NotFound(w, r)
		return
	}
	metrics.Write(w)
}
