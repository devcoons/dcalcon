package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/storage"
)

func Admin(ctx context.Context, store *storage.DB, cfg config.Config, log *slog.Logger) error {
	n, err := store.UserCount(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.Bootstrap.AdminUsername == "" || cfg.Bootstrap.AdminPassword == "" {
		log.Warn("database is empty; set DCALCON_ADMIN_USERNAME and DCALCON_ADMIN_PASSWORD to create the first admin")
		return nil
	}
	if err := rejectWeakBootstrap(cfg.Bootstrap.AdminPassword); err != nil {
		log.Error("refusing to bootstrap admin", "err", err)
		return err
	}
	email := cfg.Bootstrap.AdminUsername + "@localhost"
	u, err := store.CreateUser(ctx, cfg.Bootstrap.AdminUsername, email, cfg.Bootstrap.AdminPassword, "Administrator", "admin", "UTC")
	if err != nil {
		return err
	}
	log.Info("bootstrapped admin user", "username", u.Username)
	return nil
}

func rejectWeakBootstrap(password string) error {
	p := strings.TrimSpace(password)
	if len(p) < 8 {
		return fmt.Errorf("admin password must be at least 8 characters")
	}
	switch strings.ToLower(p) {
	case "changeme", "password", "admin", "admin123", "dcalcon":
		return fmt.Errorf("admin password %q is not allowed; set DCALCON_ADMIN_PASSWORD to a unique secret", p)
	}
	return nil
}
