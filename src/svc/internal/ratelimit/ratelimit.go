package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	max     int
	window  time.Duration
	lockout time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	fails       []time.Time
	lockedUntil time.Time
}

func New(max int, window, lockout time.Duration) *Limiter {
	if max <= 0 {
		max = 8
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	if lockout <= 0 {
		lockout = 15 * time.Minute
	}
	return &Limiter{max: max, window: window, lockout: lockout, buckets: map[string]*bucket{}}
}

func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	if l == nil {
		return true, 0
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked(now)
	b := l.buckets[key]
	if b == nil {
		return true, 0
	}
	if !b.lockedUntil.IsZero() {
		if now.Before(b.lockedUntil) {
			return false, b.lockedUntil.Sub(now)
		}
		b.lockedUntil = time.Time{}
		b.fails = nil
		return true, 0
	}
	b.fails = prune(b.fails, now.Add(-l.window))
	return true, 0
}

func (l *Limiter) Fail(key string) (locked bool, retryAfter time.Duration) {
	if l == nil {
		return false, 0
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b == nil {
		b = &bucket{}
		l.buckets[key] = b
	}
	b.fails = append(prune(b.fails, now.Add(-l.window)), now)
	if len(b.fails) >= l.max {
		b.lockedUntil = now.Add(l.lockout)
		return true, l.lockout
	}
	return false, 0
}

func (l *Limiter) Hit(key string) (ok bool, retryAfter time.Duration) {
	ok, retryAfter = l.Allow(key)
	if !ok {
		return false, retryAfter
	}
	locked, retry := l.Fail(key)
	if locked {
		return false, retry
	}
	return true, 0
}

func (l *Limiter) Success(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}

func (l *Limiter) gcLocked(now time.Time) {
	if len(l.buckets) < 512 {
		return
	}
	cutoff := now.Add(-l.window)
	for k, b := range l.buckets {
		if now.Before(b.lockedUntil) {
			continue
		}
		b.fails = prune(b.fails, cutoff)
		if len(b.fails) == 0 {
			delete(l.buckets, k)
		}
	}
}

func prune(in []time.Time, cutoff time.Time) []time.Time {
	out := in[:0]
	for _, t := range in {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}
