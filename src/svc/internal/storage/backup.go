package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (db *DB) Backup(ctx context.Context, dest string) error {
	if dest == "" {
		return fmt.Errorf("backup destination is required")
	}
	if dir := filepath.Dir(dest); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("mkdir backup dir: %w", err)
		}
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup destination already exists: %s", dest)
	}
	_, err := db.SQL.ExecContext(ctx, `VACUUM INTO ?`, dest)
	return err
}

func (db *DB) Checkpoint(ctx context.Context) error {
	_, err := db.SQL.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

func IsSQLiteFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var hdr [16]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return fmt.Errorf("read sqlite header: %w", err)
	}
	if string(hdr[:]) != "SQLite format 3\x00" {
		return fmt.Errorf("not a SQLite database: %s", path)
	}
	return nil
}

func CopyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if dir := filepath.Dir(dest); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return out.Close()
}

func PruneBackups(dir string, keep int) error {
	if keep <= 0 {
		keep = 14
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "dcalcon-") && strings.HasSuffix(n, ".db") {
			names = append(names, n)
		}
	}
	if len(names) <= keep {
		return nil
	}
	// names from ReadDir are lexical; timestamps in the filename sort oldest-first
	drop := names[:len(names)-keep]
	for _, n := range drop {
		_ = os.Remove(filepath.Join(dir, n))
	}
	return nil
}

// RunBackupHook runs an operator program with the snapshot path as argv[1].
func RunBackupHook(ctx context.Context, hook, dest string) error {
	hook = strings.TrimSpace(hook)
	if hook == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hook, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("backup hook: %w", err)
		}
		return fmt.Errorf("backup hook: %w (%s)", err, msg)
	}
	return nil
}
