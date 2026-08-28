package httpx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIPIgnoresSpoofedXFF(t *testing.T) {
	h := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, ClientIP(r))
	}), Options{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:4444"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "203.0.113.9" {
		t.Fatalf("untrusted peer must ignore XFF, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:9"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "198.51.100.7" {
		t.Fatalf("loopback may use XFF, got %q", rec.Body.String())
	}

	h = Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, ClientIP(r))
	}), Options{TrustedProxies: []string{"10.0.0.0/8"}})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:443"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.7")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "198.51.100.7" {
		t.Fatalf("trusted proxy must use rightmost XFF hop, got %q", rec.Body.String())
	}
}

func TestRequestIDRejectsJunk(t *testing.T) {
	h := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Options{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "ok\r\nX-Injected: 1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	got := rec.Header().Get("X-Request-ID")
	if got == "" || strings.ContainsAny(got, "\r\n ") {
		t.Fatalf("request id %q", got)
	}
}

func TestOriginGuardCrossSiteCookie(t *testing.T) {
	h := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Options{AllowedOrigins: []string{"http://cal.example.test"}})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "abc"})
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestWriteDownloadForcesOctetStreamForHTML(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteDownload(rec, "note.html", "text/html", []byte("<html><script>alert(1)</script>"))
	if rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("type %s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition %s", rec.Header().Get("Content-Disposition"))
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache %s", rec.Header().Get("Cache-Control"))
	}
}

func TestAllowMetrics(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !AllowMetrics(r, "") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h := Wrap(inner, Options{})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback metrics %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:9"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback peer still serves metrics, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "10.0.0.8:9"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("spoofed loopback XFF must not open metrics, got %d", rec.Code)
	}
}

func TestMaxBodyAllowsUserBackupUpload(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	h := Wrap(inner, Options{MaxBody: 10 << 20})
	body := bytes.Repeat([]byte("x"), 11<<20)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/backup", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("backup upload want 204 got %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/me/backup/export", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("export JSON still capped, got %d", rec.Code)
	}
}
