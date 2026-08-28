package config_test

import (
	"strings"
	"testing"

	"github.com/devcoons/dcalcon/internal/config"
)

func TestHTTPSForcesSecureCookies(t *testing.T) {
	t.Setenv("DCALCON_PUBLIC_URL", "https://cal.example.com")
	t.Setenv("DCALCON_SESSION_SECURE", "false")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.SessionSecure {
		t.Fatal("https public URL should force SessionSecure")
	}
	found := false
	for _, o := range cfg.AllowedOrigins() {
		if strings.EqualFold(o, "https://cal.example.com") {
			found = true
		}
	}
	if !found {
		t.Fatalf("origins %v", cfg.AllowedOrigins())
	}
}

func TestSchedulingDomain(t *testing.T) {
	t.Setenv("DCALCON_PUBLIC_URL", "http://localhost")
	t.Setenv("DCALCON_SCHEDULING_DOMAIN", " Cal.Example ")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchedulingDomain != "cal.example" {
		t.Fatalf("got %q", cfg.SchedulingDomain)
	}
}

func TestBackupHookEnv(t *testing.T) {
	t.Setenv("DCALCON_PUBLIC_URL", "http://localhost")
	t.Setenv("DCALCON_BACKUP_HOOK", "/usr/local/bin/dcalcon-offbox")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Hook != "/usr/local/bin/dcalcon-offbox" {
		t.Fatalf("hook %q", cfg.Backup.Hook)
	}
}

func TestTrustedProxiesRejectsJunk(t *testing.T) {
	t.Setenv("DCALCON_PUBLIC_URL", "http://localhost")
	t.Setenv("DCALCON_TRUSTED_PROXIES", "not-a-cidr")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected invalid trusted proxy")
	}
}
