package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/devcoons/dcalcon/internal/limits"
	"golang.org/x/crypto/bcrypt"
)

type AppPassword struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

func (db *DB) ListAppPasswords(ctx context.Context, userID int64) ([]AppPassword, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, name, prefix, created_at, COALESCE(last_used_at,'')
		FROM app_passwords WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AppPassword, 0)
	for rows.Next() {
		var p AppPassword
		if err := rows.Scan(&p.ID, &p.Name, &p.Prefix, &p.CreatedAt, &p.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func NewAppPasswordSecret() (plain, prefix string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	hexed := hex.EncodeToString(b)
	plain = "dcc_" + hexed
	prefix = "dcc_" + hexed[:8]
	return plain, prefix, nil
}

func (db *DB) CreateAppPassword(ctx context.Context, userID int64, name string) (*AppPassword, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "DAV client"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	n, err := db.CountAppPasswords(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if n >= limits.MaxAppPasswords {
		return nil, "", ErrConflict
	}
	plain, prefix, err := NewAppPasswordSecret()
	if err != nil {
		return nil, "", err
	}
	hash, err := HashPassword(plain)
	if err != nil {
		return nil, "", err
	}
	res, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO app_passwords (user_id, name, prefix, password_hash)
		VALUES (?, ?, ?, ?)`, userID, name, prefix, hash)
	if err != nil {
		return nil, "", err
	}
	id, _ := res.LastInsertId()
	list, err := db.ListAppPasswords(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], plain, nil
		}
	}
	return &AppPassword{ID: id, Name: name, Prefix: prefix}, plain, nil
}

func (db *DB) CountAppPasswords(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(1) FROM app_passwords WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

func (db *DB) DeleteAppPassword(ctx context.Context, userID, id int64) error {
	res, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM app_passwords WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) AuthenticateDAV(ctx context.Context, username, password string) (*User, error) {
	if u, err := db.Authenticate(ctx, username, password); err == nil {
		_, _, enabled, tErr := db.TOTPState(ctx, u.ID)
		if tErr == nil && enabled {
			return nil, ErrUnauthorized
		}
		return u, nil
	}
	return db.authenticateAppPassword(ctx, username, password)
}

func (db *DB) authenticateAppPassword(ctx context.Context, username, password string) (*User, error) {
	u, err := db.UserByUsername(ctx, username)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if u.Status != "active" {
		return nil, ErrUnauthorized
	}
	q := `SELECT id, password_hash FROM app_passwords WHERE user_id = ?`
	args := []any{u.ID}
	if strings.HasPrefix(password, "dcc_") && len(password) >= 12 {
		q += ` AND prefix = ?`
		args = append(args, password[:12])
	}
	rows, err := db.conn(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, ErrUnauthorized
	}
	type row struct {
		id   int64
		hash string
	}
	var candidates []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.hash); err != nil {
			_ = rows.Close()
			return nil, ErrUnauthorized
		}
		candidates = append(candidates, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, ErrUnauthorized
	}
	_ = rows.Close()
	for _, r := range candidates {
		if err := bcrypt.CompareHashAndPassword([]byte(r.hash), []byte(password)); err == nil {
			_, _ = db.conn(ctx).ExecContext(ctx, `UPDATE app_passwords SET last_used_at = ? WHERE id = ?`, nowUTC(), r.id)
			return u, nil
		}
	}
	return nil, ErrUnauthorized
}

func IsAppPasswordSecret(s string) bool {
	return strings.HasPrefix(s, "dcc_") && len(s) >= 20
}
