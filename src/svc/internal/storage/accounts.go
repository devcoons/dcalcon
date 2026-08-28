package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type OAuthState struct {
	State        string
	UserID       int64
	Provider     string
	Origin       string
	CodeVerifier string
	ExpiresAt    string
}

func (db *DB) ListConnectedAccounts(ctx context.Context, userID int64) ([]ConnectedAccount, error) {
	rows, err := db.conn(ctx).QueryContext(ctx, `
		SELECT id, provider, email, status, scopes, last_error
		FROM connected_accounts WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectedAccount
	for rows.Next() {
		var a ConnectedAccount
		var last sql.NullString
		if err := rows.Scan(&a.ID, &a.Provider, &a.Email, &a.Status, &a.Scopes, &last); err != nil {
			return nil, err
		}
		a.LastError = last.String
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) ConnectedAccountByID(ctx context.Context, userID, id int64) (*ConnectedAccount, error) {
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, provider, email, status, scopes, last_error, token_ciphertext, token_nonce
		FROM connected_accounts WHERE user_id = ? AND id = ?`, userID, id)
	a, err := scanConnected(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (db *DB) UpsertConnectedAccount(ctx context.Context, userID int64, provider, email, status, scopes string, ciphertext, nonce []byte) (*ConnectedAccount, error) {
	now := nowUTC()
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO connected_accounts (user_id, provider, email, status, scopes, token_ciphertext, token_nonce, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?)
		ON CONFLICT(user_id, provider, email) DO UPDATE SET
			status = excluded.status,
			scopes = excluded.scopes,
			token_ciphertext = excluded.token_ciphertext,
			token_nonce = excluded.token_nonce,
			last_error = '',
			updated_at = excluded.updated_at`,
		userID, provider, email, status, scopes, ciphertext, nonce, now, now)
	if err != nil {
		return nil, err
	}
	row := db.conn(ctx).QueryRowContext(ctx, `
		SELECT id, provider, email, status, scopes, last_error, token_ciphertext, token_nonce
		FROM connected_accounts WHERE user_id = ? AND provider = ? AND email = ?`, userID, provider, email)
	return scanConnected(row.Scan)
}

func (db *DB) SaveConnectedTokens(ctx context.Context, userID, id int64, ciphertext, nonce []byte) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE connected_accounts
		SET token_ciphertext = ?, token_nonce = ?, status = 'connected', last_error = '', updated_at = ?
		WHERE user_id = ? AND id = ?`, ciphertext, nonce, nowUTC(), userID, id)
	return err
}

func (db *DB) SetConnectedAccountError(ctx context.Context, userID, id int64, msg string) error {
	_, err := db.conn(ctx).ExecContext(ctx, `
		UPDATE connected_accounts SET status = 'error', last_error = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`, msg, nowUTC(), userID, id)
	return err
}

func (db *DB) DeleteConnectedAccount(ctx context.Context, userID, id int64) error {
	res, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM connected_accounts WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) PutOAuthState(ctx context.Context, st OAuthState) error {
	_, _ = db.conn(ctx).ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at < ?`, nowUTC())
	_, err := db.conn(ctx).ExecContext(ctx, `
		INSERT INTO oauth_states (state, user_id, provider, origin, code_verifier, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, st.State, st.UserID, st.Provider, st.Origin, st.CodeVerifier, st.ExpiresAt)
	return err
}

func (db *DB) TakeOAuthState(ctx context.Context, state string) (*OAuthState, error) {
	var st OAuthState
	err := db.WithTx(ctx, func(ctx context.Context) error {
		row := db.conn(ctx).QueryRowContext(ctx, `
			SELECT state, user_id, provider, origin, code_verifier, expires_at
			FROM oauth_states WHERE state = ?`, state)
		if err := row.Scan(&st.State, &st.UserID, &st.Provider, &st.Origin, &st.CodeVerifier, &st.ExpiresAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if _, err := db.conn(ctx).ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
			return err
		}
		if exp, err := time.Parse(time.RFC3339Nano, st.ExpiresAt); err == nil && time.Now().After(exp) {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func scanConnected(scan func(dest ...any) error) (*ConnectedAccount, error) {
	var a ConnectedAccount
	var last sql.NullString
	if err := scan(&a.ID, &a.Provider, &a.Email, &a.Status, &a.Scopes, &last, &a.Cipher, &a.Nonce); err != nil {
		return nil, err
	}
	a.LastError = last.String
	return &a, nil
}
