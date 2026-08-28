//go:build unix

package storage

import (
	"fmt"
	"os"
	"syscall"
)

func HoldRuntimeLock(sqlitePath string) (*RuntimeLock, error) {
	if sqlitePath == "" || sqlitePath == ":memory:" || sqlitePath == "file::memory:" {
		return &RuntimeLock{}, nil
	}
	f, err := os.OpenFile(lockPath(sqlitePath), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("runtime lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("database lock is exclusive (a restore may be running)")
	}
	return &RuntimeLock{f: f}, nil
}

func (l *RuntimeLock) Close() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}

func ErrIfRuntimeLocked(sqlitePath string) error {
	if sqlitePath == "" {
		return nil
	}
	if _, err := os.Stat(sqlitePath); os.IsNotExist(err) {
		return nil
	}
	f, err := os.OpenFile(lockPath(sqlitePath), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("database is in use; stop dcalcon before restore")
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return nil
}
