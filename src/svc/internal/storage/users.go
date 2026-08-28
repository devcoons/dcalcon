package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

var ErrNotFound = errors.New("not found")
var ErrUnauthorized = errors.New("unauthorized")
var ErrConflict = errors.New("conflict")

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Timezone    string `json:"timezone"`
	CreatedAt   string `json:"created_at"`
	TOTPEnabled bool   `json:"totp_enabled"`
}

type DirectoryUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	LocalEmail  string `json:"local_email,omitempty"`
}

const userCols = `id, username, email, display_name, role, status, timezone, created_at, totp_enabled`

func scanUser(scan func(dest ...any) error) (*User, error) {
	u := &User{}
	var totp int
	if err := scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.Timezone, &u.CreatedAt, &totp); err != nil {
		return nil, err
	}
	u.TOTPEnabled = totp == 1
	return u, nil
}

type ImportantDatesSettings struct {
	Enabled              bool     `json:"enabled"`
	IncludeBirthdays     bool     `json:"include_birthdays"`
	IncludeAnniversaries bool     `json:"include_anniversaries"`
	AlarmOffsets         []string `json:"alarm_offsets"`
}

func (db *DB) Authenticate(ctx context.Context, username, password string) (*User, error) {
	u, hash, err := db.userWithHash(ctx, username)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if u.Status != "active" {
		return nil, ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrUnauthorized
	}
	return u, nil
}

func (db *DB) userWithHash(ctx context.Context, username string) (*User, string, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, display_name, role, status, timezone, created_at
		FROM users WHERE username = ? COLLATE NOCASE`, username)
	u := &User{}
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &hash, &u.DisplayName, &u.Role, &u.Status, &u.Timezone, &u.CreatedAt); err != nil {
		return nil, "", err
	}
	return u, hash, nil
}

func (db *DB) UserByID(ctx context.Context, id int64) (*User, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (db *DB) UserByUsername(ctx context.Context, username string) (*User, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ? COLLATE NOCASE`, username)
	u, err := scanUser(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (db *DB) UserByEmail(ctx context.Context, email string) (*User, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ? COLLATE NOCASE`, strings.TrimSpace(email))
	u, err := scanUser(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (db *DB) UserCount(ctx context.Context) (int, error) {
	var n int
	err := db.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (db *DB) Directory(ctx context.Context, exceptUserID int64) ([]DirectoryUser, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, username, display_name FROM users
		WHERE status = 'active' AND id != ?
		ORDER BY username`, exceptUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DirectoryUser
	for rows.Next() {
		var u DirectoryUser
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (db *DB) CreateUser(ctx context.Context, username, email, password, displayName, role, timezone string) (*User, error) {
	if role == "" {
		role = "user"
	}
	if timezone == "" {
		timezone = "UTC"
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if displayName == "" {
		displayName = username
	}
	res, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO users (username, email, password_hash, display_name, role, timezone)
		VALUES (?, ?, ?, ?, ?, ?)`, username, email, hash, displayName, role, timezone)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrConflict
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := db.ProvisionUserDefaults(ctx, id, username); err != nil {
		return nil, err
	}
	return db.UserByID(ctx, id)
}

func (db *DB) UpdateProfile(ctx context.Context, id int64, displayName, email, timezone string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE users SET display_name = ?, email = ?, timezone = ?, updated_at = ? WHERE id = ?`,
		displayName, email, timezone, nowUTC(), id)
	return err
}

func (db *DB) SetPassword(ctx context.Context, id int64, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = db.conn(ctx).ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, nowUTC(), id)
	return err
}

func (db *DB) AdminUpdateUser(ctx context.Context, id int64, email, displayName, role, status, timezone string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE users SET email = ?, display_name = ?, role = ?, status = ?, timezone = ?, updated_at = ?
		WHERE id = ?`, email, displayName, role, status, timezone, nowUTC(), id)
	return err
}

func (db *DB) ActiveAdminCount(ctx context.Context) (int, error) {
	var n int
	err := db.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&n)
	return n, err
}

func (db *DB) DeleteSessionsForUser(ctx context.Context, userID int64) error {
	_, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (db *DB) ProvisionUserDefaults(ctx context.Context, userID int64, username string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO calendars (user_id, slug, name, description, kind)
		VALUES (?, 'personal', 'Personal', 'Default calendar', 'personal')`, userID)
	if err != nil {
		return err
	}
	if err := db.EnsureSchedulingCollections(ctx, userID); err != nil {
		return err
	}
	_, err = db.conn(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO addressbooks (user_id, slug, name, description)
		VALUES (?, 'contacts', 'Contacts', 'Default address book')`, userID)
	if err != nil {
		return err
	}
	if err := db.EnsurePeopleBook(ctx, userID); err != nil {
		return err
	}
	_, err = db.conn(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO important_dates_settings (user_id) VALUES (?)`, userID)
	_ = username
	return err
}

func (db *DB) EnsurePeopleBook(ctx context.Context, userID int64) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO addressbooks (user_id, slug, name, description, read_only)
		VALUES (?, 'people', 'People on this server', 'Other dCalCon users. Invite them with their local calendar address.', 1)`, userID)
	return err
}

func (db *DB) EnsureSchedulingCollections(ctx context.Context, userID int64) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO calendars (user_id, slug, name, description, color, kind, read_only)
		VALUES (?, 'inbox', 'Schedule Inbox', 'CalDAV scheduling inbox (RFC 6638)', '#64748B', 'inbox', 1)`, userID)
	if err != nil {
		return err
	}
	_, err = db.conn(ctx).ExecContext(ctx, `
		INSERT OR IGNORE INTO calendars (user_id, slug, name, description, color, kind, read_only)
		VALUES (?, 'outbox', 'Schedule Outbox', 'CalDAV scheduling outbox (RFC 6638)', '#94A3B8', 'outbox', 0)`, userID)
	return err
}

func (db *DB) CreateSession(ctx context.Context, id string, userID int64, ttl time.Duration) error {
	exp := time.Now().UTC().Add(ttl).Format("2006-01-02T15:04:05.000Z")
	_, err := db.conn(ctx).ExecContext(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, hashToken(id), userID, exp)
	return err
}

func (db *DB) DeleteSession(ctx context.Context, id string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM sessions WHERE id = ? OR id = ?`, hashToken(id), id)
	return err
}

func (db *DB) TOTPState(ctx context.Context, userID int64) (secret, pending string, enabled bool, err error) {
	var en int
	err = db.conn(ctx).QueryRowContext(ctx, `
		SELECT totp_secret, totp_pending, totp_enabled FROM users WHERE id = ?`, userID).Scan(&secret, &pending, &en)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, ErrNotFound
	}
	return secret, pending, en == 1, err
}

func (db *DB) SetTOTPPending(ctx context.Context, userID int64, pending string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE users SET totp_pending = ?, updated_at = ? WHERE id = ? AND totp_enabled = 0`,
		pending, nowUTC(), userID)
	return err
}

func (db *DB) EnableTOTP(ctx context.Context, userID int64, secret string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE users SET totp_secret = ?, totp_pending = '', totp_enabled = 1, updated_at = ? WHERE id = ?`,
		secret, nowUTC(), userID)
	return err
}

func (db *DB) DisableTOTP(ctx context.Context, userID int64) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE users SET totp_secret = '', totp_pending = '', totp_enabled = 0, updated_at = ? WHERE id = ?`,
		nowUTC(), userID)
	return err
}

func (db *DB) UserBySession(ctx context.Context, sessionID string) (*User, error) {
	if sessionID == "" {
		return nil, ErrUnauthorized
	}
	u, err := db.userBySessionID(ctx, hashToken(sessionID))
	if err == nil {
		return u, nil
	}
	u, err = db.userBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	_, _ = db.conn(ctx).ExecContext(ctx, `UPDATE sessions SET id = ? WHERE id = ?`, hashToken(sessionID), sessionID)
	return u, nil
}

func (db *DB) userBySessionID(ctx context.Context, storedID string) (*User, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT u.id, u.username, u.email, u.display_name, u.role, u.status, u.timezone, u.created_at, u.totp_enabled, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, storedID)
	u := &User{}
	var exp string
	var totp int
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.Timezone, &u.CreatedAt, &totp, &exp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	u.TOTPEnabled = totp == 1
	t, err := time.Parse("2006-01-02T15:04:05.000Z", exp)
	if err != nil || time.Now().UTC().After(t) || u.Status != "active" {
		return nil, ErrUnauthorized
	}
	return u, nil
}

func (db *DB) GetImportantDates(ctx context.Context, userID int64) (*ImportantDatesSettings, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT enabled, include_birthdays, include_anniversaries, alarm_offsets_json
		FROM important_dates_settings WHERE user_id = ?`, userID)
	s := &ImportantDatesSettings{}
	var enabled, bday, ann int
	var alarms string
	if err := row.Scan(&enabled, &bday, &ann, &alarms); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &ImportantDatesSettings{IncludeBirthdays: true, IncludeAnniversaries: true, AlarmOffsets: []string{"-P1D"}}, nil
		}
		return nil, err
	}
	s.Enabled = enabled == 1
	s.IncludeBirthdays = bday == 1
	s.IncludeAnniversaries = ann == 1
	s.AlarmOffsets = parseJSONStringSlice(alarms)
	if len(s.AlarmOffsets) == 0 {
		s.AlarmOffsets = []string{"-P1D"}
	}
	return s, nil
}

func (db *DB) SaveImportantDates(ctx context.Context, userID int64, s ImportantDatesSettings) error {
	alarms := `["-P1D"]`
	if len(s.AlarmOffsets) > 0 {
		alarms = fmt.Sprintf("[%s]", quoteJoin(s.AlarmOffsets))
	}
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO important_dates_settings (user_id, enabled, include_birthdays, include_anniversaries, alarm_offsets_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			enabled = excluded.enabled,
			include_birthdays = excluded.include_birthdays,
			include_anniversaries = excluded.include_anniversaries,
			alarm_offsets_json = excluded.alarm_offsets_json,
			updated_at = excluded.updated_at`,
		userID, boolInt(s.Enabled), boolInt(s.IncludeBirthdays), boolInt(s.IncludeAnniversaries), alarms, nowUTC())
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func quoteJoin(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = `"` + strings.ReplaceAll(s, `"`, ``) + `"`
	}
	return strings.Join(parts, ",")
}

func parseJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
