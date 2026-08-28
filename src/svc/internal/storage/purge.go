package storage

import (
	"context"
	"time"
)

type PurgeStats struct {
	Sessions int
	OAuth    int
	Recovery int
}

func (db *DB) PurgeExpired(ctx context.Context) (PurgeStats, error) {
	var s PurgeStats
	now := nowUTC()
	res, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now)
	if err != nil {
		return s, err
	}
	n, _ := res.RowsAffected()
	s.Sessions = int(n)

	res, err = db.conn(ctx).ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at < ?`, now)
	if err != nil {
		return s, err
	}
	n, _ = res.RowsAffected()
	s.OAuth = int(n)

	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	res, err = db.conn(ctx).ExecContext(ctx, `
		DELETE FROM password_reset_tokens
		WHERE expires_at < ? OR (used_at IS NOT NULL AND used_at < ?)`, now, cutoff)
	if err != nil {
		return s, err
	}
	n, _ = res.RowsAffected()
	s.Recovery = int(n)
	return s, nil
}
