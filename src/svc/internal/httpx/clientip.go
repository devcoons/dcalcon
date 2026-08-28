package httpx

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

const clientIPKey ctxKey = 2

func RemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ClientIP(r *http.Request) string {
	if s, ok := r.Context().Value(clientIPKey).(string); ok && s != "" {
		return s
	}
	return ResolveClientIP(r, nil)
}

func ParseTrustedProxies(cidrs []string) ([]*net.IPNet, error) {
	out := loopbackNets()
	seen := map[string]bool{}
	for _, n := range out {
		seen[n.String()] = true
	}
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		n, err := parseProxyCIDR(raw)
		if err != nil {
			return nil, err
		}
		if seen[n.String()] {
			continue
		}
		seen[n.String()] = true
		out = append(out, n)
	}
	return out, nil
}

func parseProxyCIDR(s string) (*net.IPNet, error) {
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		return n, err
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, &net.ParseError{Type: "IP address", Text: s}
	}
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}

func loopbackNets() []*net.IPNet {
	var out []*net.IPNet
	for _, cidr := range []string{"127.0.0.0/8", "::1/128"} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func trustedIP(ip string, nets []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() {
		return true
	}
	for _, n := range nets {
		if n != nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}

func ResolveClientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := RemoteIP(r)
	if !trustedIP(peer, trusted) {
		return peer
	}
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		parts := strings.Split(x, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(parts[i])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if x := strings.TrimSpace(r.Header.Get("X-Real-IP")); x != "" {
		if net.ParseIP(x) != nil {
			return x
		}
	}
	return peer
}

func withClientIP(next http.Handler, trusted []*net.IPNet) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ResolveClientIP(r, trusted)
		ctx := context.WithValue(r.Context(), clientIPKey, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) < 8 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

func AllowMetrics(r *http.Request, token string) bool {
	if token != "" {
		got := bearerToken(r)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
			return true
		}
	}
	ip := net.ParseIP(RemoteIP(r))
	return ip != nil && ip.IsLoopback()
}
