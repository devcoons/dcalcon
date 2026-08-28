package storage

import (
	"os"
)

// Restore replaces dest with a validated SQLite snapshot. It refuses if a
// runtime lock is held (dcalcon serve / split processes). The previous file
// is moved to dest+".pre-restore".
func Restore(src, dest string) (previous string, err error) {
	if err := IsSQLiteFile(src); err != nil {
		return "", err
	}
	if err := ErrIfRuntimeLocked(dest); err != nil {
		return "", err
	}
	tmp := dest + ".incoming"
	_ = os.Remove(tmp)
	if err := CopyFile(src, tmp); err != nil {
		return "", err
	}
	if _, err := os.Stat(dest); err == nil {
		bak := dest + ".pre-restore"
		_ = os.Remove(bak)
		if err := os.Rename(dest, bak); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		previous = bak
	}
	if err := os.Rename(tmp, dest); err != nil {
		return previous, err
	}
	return previous, nil
}
