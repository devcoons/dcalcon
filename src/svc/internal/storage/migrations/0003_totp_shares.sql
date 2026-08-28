-- Authenticator (TOTP) and local calendar sharing.

ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN totp_pending TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS calendar_shares (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  calendar_id      INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  grantee_user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  access           TEXT NOT NULL CHECK (access IN ('read', 'write')),
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (calendar_id, grantee_user_id)
);

CREATE INDEX IF NOT EXISTS idx_shares_grantee ON calendar_shares(grantee_user_id);
