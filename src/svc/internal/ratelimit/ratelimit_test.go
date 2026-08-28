package ratelimit

import (
	"testing"
	"time"
)

func TestLockout(t *testing.T) {
	l := New(3, time.Minute, 50*time.Millisecond)
	key := "login:127.0.0.1:alice"
	for i := 0; i < 2; i++ {
		ok, _ := l.Allow(key)
		if !ok {
			t.Fatal("should allow before threshold")
		}
		l.Fail(key)
	}
	ok, _ := l.Allow(key)
	if !ok {
		t.Fatal("third attempt should still be allowed")
	}
	locked, retry := l.Fail(key)
	if !locked || retry <= 0 {
		t.Fatalf("expected lockout, locked=%v retry=%s", locked, retry)
	}
	ok, _ = l.Allow(key)
	if ok {
		t.Fatal("locked key must be denied")
	}
	time.Sleep(60 * time.Millisecond)
	ok, _ = l.Allow(key)
	if !ok {
		t.Fatal("lockout should have expired")
	}
	l.Success(key)
	ok, _ = l.Allow(key)
	if !ok {
		t.Fatal("success should clear the bucket")
	}
}
