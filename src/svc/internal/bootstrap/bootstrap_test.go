package bootstrap_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/devcoons/dcalcon/internal/bootstrap"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/storage"
)

func TestRejectChangeme(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Default()
	cfg.Bootstrap.AdminUsername = "admin"
	cfg.Bootstrap.AdminPassword = "changeme"
	err = bootstrap.Admin(t.Context(), db, cfg, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err == nil {
		t.Fatal("expected changeme to be rejected")
	}
	n, _ := db.UserCount(t.Context())
	if n != 0 {
		t.Fatalf("no user should have been created, got %d", n)
	}
	cfg.Bootstrap.AdminPassword = "dcalcon-dev-pass"
	if err := bootstrap.Admin(t.Context(), db, cfg, slog.New(slog.NewTextHandler(os.Stderr, nil))); err != nil {
		t.Fatal(err)
	}
}
