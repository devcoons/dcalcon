package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func (db *DB) PasswordHash(ctx context.Context, userID int64) (string, error) {
	var hash string
	err := db.conn(ctx).QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return hash, err
}

func ValidPasswordHash(hash string) bool {
	hash = strings.TrimSpace(hash)
	if !strings.HasPrefix(hash, "$2") {
		return false
	}
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost >= bcrypt.MinCost
}

func (db *DB) SetPasswordHash(ctx context.Context, userID int64, hash string) error {
	hash = strings.TrimSpace(hash)
	if !ValidPasswordHash(hash) {
		return fmt.Errorf("invalid password hash")
	}
	_, err := db.conn(ctx).ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, nowUTC(), userID)
	return err
}

func (db *DB) RestoreTOTP(ctx context.Context, userID int64, secret string, enabled bool) error {
	secret = strings.TrimSpace(secret)
	if !enabled {
		return db.DisableTOTP(ctx, userID)
	}
	if secret == "" {
		return fmt.Errorf("authenticator secret missing")
	}
	return db.EnableTOTP(ctx, userID, secret)
}

type AppPasswordBackup struct {
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	Hash      string `json:"password_hash"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (db *DB) ListAppPasswordBackups(ctx context.Context, userID int64) ([]AppPasswordBackup, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT name, prefix, password_hash, created_at
		FROM app_passwords WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppPasswordBackup
	for rows.Next() {
		var p AppPasswordBackup
		if err := rows.Scan(&p.Name, &p.Prefix, &p.Hash, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []AppPasswordBackup{}
	}
	return out, rows.Err()
}

func (db *DB) ReplaceAppPasswordBackups(ctx context.Context, userID int64, list []AppPasswordBackup) error {
	if _, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM app_passwords WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, p := range list {
		if !ValidPasswordHash(p.Hash) {
			continue
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = "DAV client"
		}
		prefix := strings.TrimSpace(p.Prefix)
		if prefix == "" {
			prefix = "dcc_restored"
		}
		created := strings.TrimSpace(p.CreatedAt)
		if created == "" {
			created = nowUTC()
		}
		if _, err := db.conn(ctx).ExecContext(ctx, `
			INSERT INTO app_passwords (user_id, name, prefix, password_hash, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			userID, name, prefix, p.Hash, created); err != nil {
			return err
		}
	}
	return nil
}

type ConnectedAccountBackup struct {
	Provider string `json:"provider"`
	Email    string `json:"email"`
	Status   string `json:"status"`
	Scopes   string `json:"scopes,omitempty"`
	Cipher   []byte `json:"token_ciphertext"`
	Nonce    []byte `json:"token_nonce"`
}

func (db *DB) ListConnectedAccountBackups(ctx context.Context, userID int64) ([]ConnectedAccountBackup, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT provider, email, status, scopes, token_ciphertext, token_nonce
		FROM connected_accounts WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectedAccountBackup
	for rows.Next() {
		var a ConnectedAccountBackup
		if err := rows.Scan(&a.Provider, &a.Email, &a.Status, &a.Scopes, &a.Cipher, &a.Nonce); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []ConnectedAccountBackup{}
	}
	return out, rows.Err()
}

func (db *DB) ReplaceConnectedAccountBackups(ctx context.Context, userID int64, list []ConnectedAccountBackup) error {
	if _, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM connected_accounts WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, a := range list {
		provider := strings.ToLower(strings.TrimSpace(a.Provider))
		email := strings.TrimSpace(a.Email)
		if provider == "" || email == "" {
			continue
		}
		status := strings.TrimSpace(a.Status)
		if status == "" {
			status = "connected"
		}
		now := nowUTC()
		if _, err := db.conn(ctx).ExecContext(ctx, `
			INSERT INTO connected_accounts (user_id, provider, email, status, scopes, token_ciphertext, token_nonce, last_error, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			userID, provider, email, status, a.Scopes, a.Cipher, a.Nonce, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) PutWebcalToken(ctx context.Context, userID, calendarID int64, token string) error {
	stored, err := storedWebcalToken(token)
	if err != nil {
		return err
	}
	return db.upsertWebcal(ctx, userID, calendarID, stored)
}

func (db *DB) attachmentIDTaken(ctx context.Context, publicID string) bool {
	var n int
	_ = db.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM calendar_attachments WHERE public_id = ?`, publicID).Scan(&n)
	return n > 0
}

func (db *DB) SetCalendarTimezone(ctx context.Context, id int64, tz string) error {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return nil
	}
	_, err := db.conn(ctx).ExecContext(ctx, `UPDATE calendars SET timezone = ?, updated_at = ? WHERE id = ?`, tz, nowUTC(), id)
	return err
}

func (db *DB) EnsureAddressBook(ctx context.Context, userID int64, slug, name, description string) (*AddressBook, error) {
	b, err := db.AddressBookBySlug(ctx, userID, slug)
	if err == nil {
		return b, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if name == "" {
		name = slug
	}
	_, err = db.conn(ctx).ExecContext(ctx, `
		INSERT INTO addressbooks (user_id, slug, name, description)
		VALUES (?, ?, ?, ?)`, userID, slug, name, description)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return db.AddressBookBySlug(ctx, userID, slug)
		}
		return nil, err
	}
	return db.AddressBookBySlug(ctx, userID, slug)
}
