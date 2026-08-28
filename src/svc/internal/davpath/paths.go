package davpath

import (
	"errors"
	"path"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

const (
	RootPath         = "/dav/"
	PrincipalsRoot   = "/dav/principals/"
	CalendarsRoot    = "/dav/calendars/"
	AddressBooksRoot = "/dav/addressbooks/"
)

func PrincipalPath(username string) string {
	return PrincipalsRoot + username + "/"
}

func CalendarHome(username string) string {
	return CalendarsRoot + username + "/"
}

func AddressBookHome(username string) string {
	return AddressBooksRoot + username + "/"
}

func CalendarPath(username, slug string) string {
	return CalendarHome(username) + slug + "/"
}

func AddressBookPath(username, slug string) string {
	return AddressBookHome(username) + slug + "/"
}

func ObjectPath(collectionPath, href string) string {
	return strings.TrimRight(collectionPath, "/") + "/" + strings.TrimLeft(href, "/")
}

func CalendarSlug(path, username string) string {
	prefix := CalendarHome(username)
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	slug, _, _ := strings.Cut(strings.Trim(rest, "/"), "/")
	return slug
}

func AddressBookSlug(path, username string) string {
	prefix := AddressBookHome(username)
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	slug, _, _ := strings.Cut(strings.Trim(rest, "/"), "/")
	return slug
}

func ObjectHref(objPath, collectionPath string) string {
	return strings.TrimPrefix(objPath, strings.TrimRight(collectionPath, "/")+"/")
}

func CheckObjectHref(href string) error {
	href = strings.TrimSpace(href)
	if href == "" || href == "." || href == ".." {
		return errors.New("invalid object href")
	}
	low := strings.ToLower(href)
	if strings.ContainsAny(href, `/\`) || strings.Contains(low, "%2f") || strings.Contains(low, "%5c") || strings.Contains(low, "%2e%2e") {
		return errors.New("invalid object href")
	}
	if path.Base(href) != href {
		return errors.New("invalid object href")
	}
	return nil
}

func ObjectFileHref(uid, ext string) string {
	uid = strings.TrimSpace(uid)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	href := uid + ext
	if CheckObjectHref(href) == nil {
		return href
	}
	return uuid.NewString() + ext
}

func ValidSlug(s string) bool {
	if len(s) < 1 || len(s) > 63 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}

func Slugify(slug, name string) string {
	s := strings.TrimSpace(slug)
	if s == "" {
		s = strings.TrimSpace(name)
	}
	s = strings.ToLower(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '_' || r == '-' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	if !ValidSlug(out) {
		return "calendar"
	}
	return out
}

func ZipSegment(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	s = path.Base(s)
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" || s == "." || s == ".." {
		return "item"
	}
	var b strings.Builder
	for _, r := range s {
		if r < 32 || r == 127 || r == '"' || unicode.Is(unicode.C, r) {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), " .")
	if out == "" || out == "." || out == ".." {
		return "item"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

func ZipPath(parts ...string) string {
	segs := make([]string, 0, len(parts))
	for _, p := range parts {
		segs = append(segs, ZipSegment(p))
	}
	return strings.Join(segs, "/")
}

func WriteXML(b *strings.Builder, s string) {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	b.WriteString(s)
}
