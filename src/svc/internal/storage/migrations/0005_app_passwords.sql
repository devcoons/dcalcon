-- App passwords for DAV Basic (separate from the dashboard password).

CREATE TABLE IF NOT EXISTS app_passwords (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  prefix        TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  last_used_at  TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_app_passwords_user ON app_passwords(user_id);
CREATE INDEX IF NOT EXISTS idx_app_passwords_prefix ON app_passwords(prefix);
