package storage

import "os"

// RuntimeLock is a shared flock on sqlitePath.lock so restore can refuse a live process.
type RuntimeLock struct {
	f *os.File
}

func lockPath(sqlitePath string) string {
	return sqlitePath + ".lock"
}
