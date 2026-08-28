package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type RecoveryMessage struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username,omitempty"`
	Email       string `json:"email"`
	RecoveryURL string `json:"-"`
	Delivered   string `json:"delivered"`
	LastError   string `json:"last_error,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func (db *DB) ReplacePasswordReset(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		now := nowUTC()
		if _, err := db.conn(ctx).ExecContext(ctx, `
			UPDATE password_reset_tokens SET used_at = ? WHERE user_id = ? AND used_at IS NULL`, now, userID); err != nil {
			return err
		}
		exp := time.Now().UTC().Add(ttl).Format("2006-01-02T15:04:05.000Z")
		_, err := db.conn(ctx).ExecContext(ctx, `
			INSERT INTO password_reset_tokens (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
			userID, tokenHash, exp)
		return err
	})
}

func (db *DB) ConsumePasswordReset(ctx context.Context, tokenHash, password string) (*User, error) {
	var out *User
	err := db.WithTx(ctx, func(ctx context.Context) error {
		row := db.conn(ctx).QueryRowContext(ctx, `
			SELECT id, user_id, expires_at, used_at FROM password_reset_tokens WHERE token_hash = ?`, tokenHash)
		var id, userID int64
		var exp, used sql.NullString
		if err := row.Scan(&id, &userID, &exp, &used); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if used.Valid && used.String != "" {
			return ErrUnauthorized
		}
		t, err := time.Parse("2006-01-02T15:04:05.000Z", exp.String)
		if err != nil || time.Now().UTC().After(t) {
			return ErrUnauthorized
		}
		res, err := db.conn(ctx).ExecContext(ctx, `
			UPDATE password_reset_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`, nowUTC(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrUnauthorized
		}
		if err := db.SetPassword(ctx, userID, password); err != nil {
			return err
		}
		if err := db.DeleteSessionsForUser(ctx, userID); err != nil {
			return err
		}
		u, err := db.UserByID(ctx, userID)
		if err != nil {
			return err
		}
		out = u
		return nil
	})
	return out, err
}

func (db *DB) InsertRecoveryOutbox(ctx context.Context, userID int64, email, delivered, lastErr string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO recovery_outbox (user_id, email, recovery_url, delivered, last_error)
		VALUES (?, ?, '', ?, ?)`, userID, email, delivered, lastErr)
	return err
}

func (db *DB) ListRecoveryOutbox(ctx context.Context, limit int) ([]RecoveryMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT o.id, o.user_id, COALESCE(u.username,''), o.email, o.recovery_url, o.delivered, COALESCE(o.last_error,''), o.created_at
		FROM recovery_outbox o
		LEFT JOIN users u ON u.id = o.user_id
		ORDER BY o.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoveryMessage
	for rows.Next() {
		var m RecoveryMessage
		if err := rows.Scan(&m.ID, &m.UserID, &m.Username, &m.Email, &m.RecoveryURL, &m.Delivered, &m.LastError, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.RecoveryURL = ""
		out = append(out, m)
	}
	return out, rows.Err()
}
