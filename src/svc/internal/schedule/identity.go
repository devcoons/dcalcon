package schedule

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/devcoons/dcalcon/internal/icsutil"
	"github.com/devcoons/dcalcon/internal/storage"
)

// LocalDomain is the reserved host for in-server calendar-user addresses.
// Generic CalDAV apps invite by EMAIL; contacts use username@this-domain so
// ATTENDEE lines route locally instead of by a person's internet mailbox.
const DefaultLocalDomain = "dcalcon.private"

var localDomain atomic.Value // string

func SetLocalDomain(d string) {
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" {
		d = DefaultLocalDomain
	}
	localDomain.Store(d)
}

func LocalDomain() string {
	v, _ := localDomain.Load().(string)
	if v == "" {
		return DefaultLocalDomain
	}
	return v
}

func LocalMailbox(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	return strings.ToLower(username) + "@" + LocalDomain()
}

func IsLocalMailbox(ident string) bool {
	_, host, ok := splitMailbox(icsutil.AddrOf(ident))
	return ok && isLocalHost(host)
}

func isLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if host == LocalDomain() {
		return true
	}
	switch host {
	case DefaultLocalDomain, "dcalcon.invalid":
		return true
	default:
		return false
	}
}

func splitMailbox(addr string) (local, host string, ok bool) {
	addr = strings.TrimSpace(addr)
	i := strings.LastIndex(addr, "@")
	if i <= 0 || i == len(addr)-1 {
		return "", "", false
	}
	return addr[:i], addr[i+1:], true
}

func Identities(u *storage.User) []string {
	if u == nil {
		return nil
	}
	out := make([]string, 0, 4)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, e := range out {
			if strings.EqualFold(e, s) {
				return
			}
		}
		out = append(out, s)
	}
	add(u.Username)
	add(u.Email)
	add(LocalMailbox(u.Username))
	return out
}

func SameUser(u *storage.User, ident string) bool {
	ident = strings.ToLower(icsutil.AddrOf(ident))
	if u == nil || ident == "" {
		return false
	}
	for _, id := range Identities(u) {
		if strings.EqualFold(id, ident) {
			return true
		}
	}
	return false
}

func FindUser(ctx context.Context, db *storage.DB, ident string) (*storage.User, error) {
	ident = icsutil.AddrOf(ident)
	if ident == "" {
		return nil, storage.ErrNotFound
	}
	if local, host, ok := splitMailbox(ident); ok && isLocalHost(host) {
		return db.UserByUsername(ctx, local)
	}
	if u, err := db.UserByUsername(ctx, ident); err == nil {
		return u, nil
	}
	return db.UserByEmail(ctx, ident)
}
