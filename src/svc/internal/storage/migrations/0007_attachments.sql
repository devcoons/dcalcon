-- Files for VEVENT / VTODO. Bytes live here, not in ICS.
-- ICS keeps ATTACH URIs with MANAGED-ID pointing at /dav/attachments/{public_id}.

CREATE TABLE IF NOT EXISTS calendar_attachments (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id     TEXT NOT NULL UNIQUE,
  calendar_id   INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  object_href   TEXT NOT NULL,
  filename      TEXT NOT NULL,
  content_type  TEXT NOT NULL DEFAULT 'application/octet-stream',
  size          INTEGER NOT NULL,
  sha256        TEXT NOT NULL DEFAULT '',
  data          BLOB NOT NULL,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY (calendar_id, object_href) REFERENCES calendar_objects(calendar_id, href) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_calendar_attachments_object
  ON calendar_attachments(calendar_id, object_href);
