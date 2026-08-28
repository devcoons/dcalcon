package userbackup

import (
	"errors"
	"strings"

	"github.com/devcoons/dcalcon/internal/storage"
)

const (
	Format   = "dcalcon.user-backup"
	Version  = 1
	KindData = "data"
	KindFull = "full"
)

var (
	ErrNotBackup   = errors.New("not a dCalCon user backup")
	ErrUnsafeZip   = errors.New("backup contains an unsafe path")
	ErrUsername    = errors.New("backup belongs to a different user")
	ErrUnsupported = errors.New("unsupported backup version")
	ErrKind        = errors.New("backup kind must be data or full")
	ErrTooLarge    = errors.New("backup is too large")
)

type Manifest struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	ExportedAt string `json:"exported_at"`
	Username   string `json:"username"`
}

type CalendarMeta struct {
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Color       string      `json:"color"`
	Kind        string      `json:"kind"`
	Timezone    string      `json:"timezone"`
	WebcalToken string      `json:"webcal_token,omitempty"`
	Items       []ObjectRef `json:"items"`
	Files       []FileRef   `json:"files"`
}

type ObjectRef struct {
	Href      string `json:"href"`
	UID       string `json:"uid,omitempty"`
	Component string `json:"component,omitempty"`
	File      string `json:"file"`
}

type FileRef struct {
	Href        string `json:"href"`
	PublicID    string `json:"public_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	File        string `json:"file"`
}

type BookMeta struct {
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Items       []ObjectRef `json:"items"`
}

type Account struct {
	Username          string                           `json:"username"`
	Email             string                           `json:"email"`
	DisplayName       string                           `json:"display_name"`
	Timezone          string                           `json:"timezone"`
	PasswordHash      string                           `json:"password_hash"`
	TOTPEnabled       bool                             `json:"totp_enabled"`
	TOTPSecret        string                           `json:"totp_secret,omitempty"`
	AppPasswords      []storage.AppPasswordBackup      `json:"app_passwords"`
	ConnectedAccounts []storage.ConnectedAccountBackup `json:"connected_accounts"`
	Shares            []ShareBackup                    `json:"shares"`
}

type ShareBackup struct {
	CalendarSlug string `json:"calendar_slug"`
	Grantee      string `json:"grantee"`
	Access       string `json:"access"`
}

type Result struct {
	Kind      string   `json:"kind"`
	Calendars int      `json:"calendars"`
	Objects   int      `json:"objects"`
	Contacts  int      `json:"contacts"`
	Files     int      `json:"files"`
	Skipped   []string `json:"skipped,omitempty"`
}

func NormalizeKind(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", KindData:
		return KindData, nil
	case KindFull:
		return KindFull, nil
	default:
		return "", ErrKind
	}
}

func skipCalendarKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "important_dates", "shared":
		return true
	default:
		return false
	}
}

func restorableCalendarKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "personal", "inbox", "outbox":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return ""
	}
}
