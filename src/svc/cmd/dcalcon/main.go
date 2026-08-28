package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/devcoons/dcalcon/internal/app"
	"github.com/devcoons/dcalcon/internal/bootstrap"
	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/schedule"
	"github.com/devcoons/dcalcon/internal/secret"
	"github.com/devcoons/dcalcon/internal/storage"
	"github.com/devcoons/dcalcon/internal/version"
	"github.com/devcoons/dcalcon/internal/worker"
	svcapi "github.com/devcoons/dcalcon/services/api"
	svccal "github.com/devcoons/dcalcon/services/caldav"
	svccard "github.com/devcoons/dcalcon/services/carddav"
	svcworker "github.com/devcoons/dcalcon/services/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	if cmd == "version" || cmd == "-v" || cmd == "--version" {
		fmt.Printf("%s %s\n", version.Name, version.Version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	if cmd == "restore" {
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: dcalcon restore <backup.db> [dest.db]\n")
			os.Exit(2)
		}
		src := os.Args[2]
		dest := cfg.SQLite.Path
		if len(os.Args) > 3 {
			dest = os.Args[3]
		}
		prev, err := storage.Restore(src, dest)
		if err != nil {
			log.Error("restore", "err", err)
			os.Exit(1)
		}
		if prev != "" {
			log.Info("previous database moved aside", "path", prev)
		}
		log.Info("restore complete", "src", src, "dest", dest)
		return
	}

	schedule.SetLocalDomain(cfg.SchedulingDomain)

	if key, err := secret.EnsureKey(cfg.Auth.TokenKey, cfg.SQLite.Path); err != nil {
		log.Error("token key", "err", err)
		os.Exit(1)
	} else if key != "" {
		cfg.Auth.TokenKey = key
	}

	store, err := storage.Open(cfg.SQLite.Path)
	if err != nil {
		log.Error("sqlite", "err", err, "path", cfg.SQLite.Path)
		os.Exit(1)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cmd == "backup" {
		dest := ""
		if len(os.Args) > 2 {
			dest = os.Args[2]
		}
		if dest == "" {
			dir := strings.TrimSpace(cfg.Backup.Dir)
			if dir == "" {
				dir = filepath.Join(filepath.Dir(cfg.SQLite.Path), "backups")
			}
			dest = filepath.Join(dir, time.Now().UTC().Format("dcalcon-20060102-150405.db"))
		}
		if err := store.Backup(ctx, dest); err != nil {
			log.Error("backup", "err", err)
			os.Exit(1)
		}
		if err := storage.RunBackupHook(ctx, cfg.Backup.Hook, dest); err != nil {
			log.Error("backup hook", "err", err, "path", dest)
			os.Exit(1)
		}
		log.Info("backup written", "path", dest)
		return
	}

	if err := bootstrap.Admin(ctx, store, cfg, log); err != nil {
		log.Error("bootstrap", "err", err)
		os.Exit(1)
	}

	if !cfg.Auth.SessionSecure {
		log.Warn("session cookies are not marked Secure; set DCALCON_PUBLIC_URL to https://… or DCALCON_SESSION_SECURE=true behind TLS")
	}

	lock, err := storage.HoldRuntimeLock(cfg.SQLite.Path)
	if err != nil {
		log.Error("runtime lock", "err", err)
		os.Exit(1)
	}
	defer lock.Close()

	if cmd != "serve" && cmd != "core" {
		log.Warn("split process mode is not a production topology: rate limits are per-process and SQLite has multiple writers; use dcalcon serve")
	}

	var runErr error
	switch cmd {
	case "serve", "core":
		app.StartWorker(ctx, store, cfg, log)
		runErr = app.Listen(ctx, cfg.HTTP.Addr, app.CombinedHandler(store, cfg), log)
	case "caldav":
		runErr = app.Listen(ctx, cfg.HTTP.Addr, svccal.New(store, cfg), log)
	case "carddav":
		runErr = app.Listen(ctx, cfg.HTTP.Addr, svccard.New(store, cfg), log)
	case "api":
		runErr = app.Listen(ctx, cfg.HTTP.Addr, svcapi.New(store, cfg), log)
	case "worker":
		go func() {
			_ = app.Listen(ctx, cfg.HTTP.Addr, svcworker.Handler(store), log)
		}()
		w := &worker.Worker{Store: store, Cfg: cfg, Log: log}
		w.Run(ctx)
	default:
		fmt.Fprintf(os.Stderr, "usage: dcalcon [serve|caldav|carddav|api|worker|backup|restore|version]\n")
		os.Exit(2)
	}
	if runErr != nil && ctx.Err() == nil {
		log.Error("server", "err", runErr)
		os.Exit(1)
	}
}
