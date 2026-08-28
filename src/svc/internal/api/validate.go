package api

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{1,31}$`)

func validUsername(s string) bool {
	return usernameRE.MatchString(s)
}

func validEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 254 {
		return false
	}
	_, err := mail.ParseAddress(s)
	return err == nil
}

func forbiddenPassword(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "changeme", "password", "admin", "admin123", "dcalcon":
		return true
	default:
		return false
	}
}

func validPassword(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 8 && n <= 128 && !forbiddenPassword(s)
}

func validRole(s string) bool {
	return s == "admin" || s == "user"
}

func validStatus(s string) bool {
	return s == "active" || s == "disabled"
}

func normalizeTimezone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "UTC"
	}
	return s
}
