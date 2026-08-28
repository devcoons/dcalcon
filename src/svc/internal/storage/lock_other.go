//go:build !unix

package storage

func HoldRuntimeLock(string) (*RuntimeLock, error) {
	return &RuntimeLock{}, nil
}

func (l *RuntimeLock) Close() {
	if l == nil {
		return
	}
	l.f = nil
}

func ErrIfRuntimeLocked(string) error {
	return nil
}
