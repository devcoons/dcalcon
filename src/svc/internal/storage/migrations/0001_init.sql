-- dCalCon canonical schema. SQLite is the system of record for users,
-- calendars, contacts, scheduling, and connected accounts.

CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
  email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  display_name  TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
  status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  timezone      TEXT NOT NULL DEFAULT 'UTC',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS calendars (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug        TEXT NOT NULL,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  color       TEXT NOT NULL DEFAULT '#E72625',
  timezone    TEXT NOT NULL DEFAULT 'UTC',
  kind        TEXT NOT NULL DEFAULT 'personal'
                CHECK (kind IN ('personal', 'important_dates', 'inbox', 'outbox', 'shared')),
  read_only   INTEGER NOT NULL DEFAULT 0,
  ctag        INTEGER NOT NULL DEFAULT 1,
  sync_token  TEXT NOT NULL DEFAULT '1',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (user_id, slug)
);

CREATE TABLE IF NOT EXISTS calendar_objects (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  href        TEXT NOT NULL,
  uid         TEXT NOT NULL,
  etag        TEXT NOT NULL,
  component   TEXT NOT NULL,
  ics         TEXT NOT NULL,
  dtstart     TEXT,
  dtend       TEXT,
  summary     TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (calendar_id, href)
);

CREATE INDEX IF NOT EXISTS idx_calendar_objects_uid ON calendar_objects(uid);
CREATE INDEX IF NOT EXISTS idx_calendar_objects_range ON calendar_objects(calendar_id, dtstart, dtend);

CREATE TABLE IF NOT EXISTS addressbooks (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug        TEXT NOT NULL,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  read_only   INTEGER NOT NULL DEFAULT 0,
  ctag        INTEGER NOT NULL DEFAULT 1,
  sync_token  TEXT NOT NULL DEFAULT '1',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (user_id, slug)
);

CREATE TABLE IF NOT EXISTS address_objects (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  addressbook_id INTEGER NOT NULL REFERENCES addressbooks(id) ON DELETE CASCADE,
  href           TEXT NOT NULL,
  uid            TEXT NOT NULL,
  etag           TEXT NOT NULL,
  vcard          TEXT NOT NULL,
  fn             TEXT,
  bday           TEXT,
  anniversary    TEXT,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (addressbook_id, href)
);

CREATE INDEX IF NOT EXISTS idx_address_objects_uid ON address_objects(uid);

CREATE TABLE IF NOT EXISTS important_dates_settings (
  user_id                INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  enabled                INTEGER NOT NULL DEFAULT 0,
  include_birthdays      INTEGER NOT NULL DEFAULT 1,
  include_anniversaries  INTEGER NOT NULL DEFAULT 1,
  -- JSON array of ISO-8601 durations before the all-day event, e.g. ["PT0S","-P1D","-P7D"]
  alarm_offsets_json     TEXT NOT NULL DEFAULT '["-P1D"]',
  updated_at             TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS schedule_items (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  collection TEXT NOT NULL CHECK (collection IN ('inbox', 'outbox')),
  method     TEXT NOT NULL,
  uid        TEXT NOT NULL,
  organizer  TEXT,
  attendee   TEXT,
  status     TEXT NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled', 'processed')),
  ics        TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_schedule_user_status ON schedule_items(user_id, status);

CREATE TABLE IF NOT EXISTS connected_accounts (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider         TEXT NOT NULL CHECK (provider IN ('google', 'microsoft', 'smtp')),
  email            TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'connected', 'error', 'revoked')),
  scopes           TEXT NOT NULL DEFAULT '',
  token_ciphertext BLOB,
  token_nonce      BLOB,
  last_error       TEXT,
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE (user_id, provider, email)
);
