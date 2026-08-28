package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/devcoons/dcalcon/internal/httpx"
	"github.com/devcoons/dcalcon/internal/metrics"
	"github.com/devcoons/dcalcon/internal/ratelimit"
	"github.com/devcoons/dcalcon/internal/storage"
)

type ctxKey int

const principalKey ctxKey = 1

const SessionCookie = "dcalcon_session"

type Principal struct {
	ID          int64
	Username    string
	Email       string
	DisplayName string
	Role        string
}

func (p Principal) IsAdmin() bool { return p.Role == "admin" }

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

func MustPrincipal(ctx context.Context) Principal {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		return Principal{}
	}
	return p
}

func NewSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Basic(store *storage.DB, realm string, lim *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			key := "dav:" + httpx.ClientIP(r) + ":" + strings.ToLower(strings.TrimSpace(user))
			if !ok || user == "" {
				challenge(w, realm)
				return
			}
			if ok, retry := lim.Allow(key); !ok {
				metrics.IncAuthLockout()
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				http.Error(w, "too many attempts", http.StatusTooManyRequests)
				return
			}
			u, err := store.AuthenticateDAV(r.Context(), user, pass)
			if err != nil {
				locked, retry := lim.Fail(key)
				metrics.IncAuthFail()
				if locked {
					metrics.IncAuthLockout()
					w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
					http.Error(w, "too many attempts", http.StatusTooManyRequests)
					return
				}
				challenge(w, realm)
				return
			}
			lim.Success(key)
			p := Principal{
				ID: u.ID, Username: u.Username, Email: u.Email,
				DisplayName: u.DisplayName, Role: u.Role,
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

func challenge(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func SessionOrBearer(store *storage.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""
			if c, err := r.Cookie(SessionCookie); err == nil {
				token = c.Value
			}
			if token == "" {
				if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
					token = strings.TrimSpace(h[7:])
				}
			}
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			u, err := store.UserBySession(r.Context(), token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			p := Principal{
				ID: u.ID, Username: u.Username, Email: u.Email,
				DisplayName: u.DisplayName, Role: u.Role,
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok || !p.IsAdmin() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
