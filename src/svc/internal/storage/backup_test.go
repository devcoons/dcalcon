package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devcoons/dcalcon/internal/storage"
)

func TestBackupAndAppPassword(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	u, err := db.CreateUser(ctx, "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	pw, secret, err := db.CreateAppPassword(ctx, u.ID, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AuthenticateDAV(ctx, "alice", secret); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Authenticate(ctx, "alice", secret); err == nil {
		t.Fatal("app password must not work for dashboard login")
	}
	list, err := db.ListAppPasswords(ctx, u.ID)
	if err != nil || len(list) != 1 || list[0].ID != pw.ID {
		t.Fatalf("list %+v %v", list, err)
	}

	dest := filepath.Join(dir, "snap.db")
	if err := db.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() == 0 {
		t.Fatalf("backup missing: %v", err)
	}

	stale := t.Context()
	_, _ = db.SQL.ExecContext(stale, `INSERT INTO sessions (id, user_id, expires_at) VALUES ('gone', ?, '2000-01-01T00:00:00.000Z')`, u.ID)
	stats, err := db.PurgeExpired(stale)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Sessions < 1 {
		t.Fatalf("expected session purge, got %+v", stats)
	}

	if err := storage.IsSQLiteFile(dest); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(dir, "not.db")
	if err := os.WriteFile(junk, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := storage.IsSQLiteFile(junk); err == nil {
		t.Fatal("expected non-sqlite file to be rejected")
	}
}

func TestRestoreAndPrune(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live.db")
	db, err := storage.Open(live)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(t.Context(), "alice", "alice@example.com", "secret-pass", "Alice", "user", "UTC"); err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(dir, "dcalcon-20260828-000000.db")
	if err := db.Backup(t.Context(), snap); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	dest := filepath.Join(dir, "restored.db")
	if _, err := storage.Restore(snap, dest); err != nil {
		t.Fatal(err)
	}
	if err := storage.IsSQLiteFile(dest); err != nil {
		t.Fatal(err)
	}

	lock, err := storage.HoldRuntimeLock(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Restore(snap, dest); err == nil {
		t.Fatal("restore must refuse a locked database")
	}
	lock.Close()

	keepDir := filepath.Join(dir, "keep")
	if err := os.MkdirAll(keepDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"dcalcon-20260101-000000.db", "dcalcon-20260102-000000.db", "dcalcon-20260103-000000.db"} {
		if err := os.WriteFile(filepath.Join(keepDir, n), []byte("SQLite format 3\x00"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.PruneBackups(keepDir, 2); err != nil {
		t.Fatal(err)
	}
	left, err := os.ReadDir(keepDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Fatalf("keep %d", len(left))
	}
}

func TestBackupHook(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "copied")
	hook := filepath.Join(dir, "hook.sh")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"" + marker + "\"\n"
	if err := os.WriteFile(hook, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(dir, "snap.db")
	if err := os.WriteFile(snap, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunBackupHook(t.Context(), "", snap); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("empty hook must not run")
	}
	if err := storage.RunBackupHook(t.Context(), hook, snap); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != snap {
		t.Fatalf("hook argv %q", got)
	}
}
