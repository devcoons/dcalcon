package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type AuditEntry struct {
	ID      int64  `json:"id"`
	At      string `json:"at"`
	ActorID int64  `json:"actor_id,omitempty"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Detail  string `json:"detail"`
}

func (db *DB) Audit(ctx context.Context, actorID int64, actor, action, detail string) {
	if db == nil || db.SQL == nil {
		return
	}
	_, _ = db.conn(ctx).ExecContext(ctx, `
		INSERT INTO audit_log (at, actor_id, actor, action, detail)
		VALUES (?, ?, ?, ?, ?)`, nowUTC(), actorID, actor, action, detail)
}

func (db *DB) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, at, COALESCE(actor_id,0), actor, action, detail
		FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.ActorID, &e.Actor, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type WebcalToken struct {
	Token      string `json:"token"`
	UserID     int64  `json:"user_id"`
	CalendarID int64  `json:"calendar_id"`
	CreatedAt  string `json:"created_at"`
}

func (db *DB) WebcalForCalendar(ctx context.Context, userID, calendarID int64) (*WebcalToken, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT token, user_id, calendar_id, created_at
		FROM webcal_tokens WHERE user_id = ? AND calendar_id = ?`, userID, calendarID)
	t := &WebcalToken{}
	if err := row.Scan(&t.Token, &t.UserID, &t.CalendarID, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (db *DB) webcalRow(ctx context.Context, token string) (*WebcalToken, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT token, user_id, calendar_id, created_at FROM webcal_tokens WHERE token = ?`, token)
	t := &WebcalToken{}
	if err := row.Scan(&t.Token, &t.UserID, &t.CalendarID, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (db *DB) WebcalByToken(ctx context.Context, token string) (*WebcalToken, error) {
	t, err := db.webcalRow(ctx, hashToken(token))
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	t, err = db.webcalRow(ctx, token)
	if err != nil {
		return nil, err
	}
	_, _ = db.conn(ctx).ExecContext(ctx, `UPDATE webcal_tokens SET token = ? WHERE token = ?`, hashToken(token), token)
	return t, nil
}

func storedWebcalToken(token string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(token))
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("invalid webcal token")
	}
	switch len(s) {
	case 36:
		return hashToken(s), nil
	case 64:
		return s, nil
	default:
		return "", fmt.Errorf("invalid webcal token")
	}
}

func (db *DB) upsertWebcal(ctx context.Context, userID, calendarID int64, stored string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO webcal_tokens (token, user_id, calendar_id, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(calendar_id) DO UPDATE SET token = excluded.token, created_at = excluded.created_at`,
		stored, userID, calendarID, nowUTC())
	return err
}

func (db *DB) RotateWebcal(ctx context.Context, userID, calendarID int64) (*WebcalToken, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(b)
	created := nowUTC()
	if err := db.upsertWebcal(ctx, userID, calendarID, hashToken(token)); err != nil {
		return nil, err
	}
	return &WebcalToken{Token: token, UserID: userID, CalendarID: calendarID, CreatedAt: created}, nil
}

func (db *DB) DeleteWebcal(ctx context.Context, userID, calendarID int64) error {
	res, err := db.conn(ctx).ExecContext(ctx, `
		DELETE FROM webcal_tokens WHERE user_id = ? AND calendar_id = ?`, userID, calendarID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) DeleteSessionsForUserExcept(ctx context.Context, userID int64, keepID string) error {
	if keepID == "" {
		return db.DeleteSessionsForUser(ctx, userID)
	}
	_, err := db.conn(ctx).ExecContext(ctx, `
		DELETE FROM sessions WHERE user_id = ? AND id != ? AND id != ?`, userID, keepID, hashToken(keepID))
	return err
}
