package app_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/internal/app"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/storage"
)

func TestReadyzMetricsOrigin(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.CreateUser(t.Context(), "admin", "admin@localhost", "changeme1", "Administrator", "admin", "UTC"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.HTTP.PublicURL = "http://cal.example.test"
	ts := httptest.NewServer(app.CombinedHandler(db, cfg))
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(raw), "ready") {
		t.Fatalf("readyz %d %s", res.StatusCode, raw)
	}

	res, err = http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(raw), "dcalcon_auth_failures_total") {
		t.Fatalf("metrics %d %s", res.StatusCode, raw)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/metrics", nil)
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("metrics uses TCP peer, not XFF; loopback want 200, got %d", res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"changeme1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("evil origin want 403, got %d", res.StatusCode)
	}
}
