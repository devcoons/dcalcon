# Limitations

## Product

- This is not yet a drop-in Baikal replacement. Run the interop tests (`go test ./internal/dav/`) and exercise DAVx⁵ / GNOME against a live HTTPS node before promising production sync.
- Recurring events are stored with RRULE. calendar-query time-range matches recurrences, and RFC 4791 `<expand/>` rewrites `calendar-data` into instances (capped). The dashboard can edit FREQ/INTERVAL/UNTIL/COUNT.
- CalDAV scheduling: each user has **Schedule Inbox** / **Schedule Outbox**. Local REST invites and CalDAV PUTs with `ATTENDEE` that map to a user deposit `METHOD:REQUEST` in the attendee inbox. **Outbox POST**, **free/busy REPORT**, CANCEL on delete, and REPLY PARTSTAT merge are implemented; `calendar-auto-schedule` is advertised. **iMIP** to external addresses is send-only. `@dcalcon.private` is never mailed.
- Sharing a whole calendar with another local user is available in the dashboard (read/write). Shared collections appear in the grantee’s calendar home as `x-share-{id}` and on calendar-proxy properties. CalDAV advertises `access-control` and maps the ACL method plus privilege/acl properties onto those same whole-calendar shares. Full RFC 3744 (per-object ACE, inherited ACEs) and CS:invite are not implemented.
- Personal calendars advertise VEVENT and VTODO. VJOURNAL is not advertised and PUT of VJOURNAL is rejected (403).
- TOTP is a **second factor** on password login. When it is enabled, DAV rejects the account password and requires an **app password**.
- Event/task **files** live in SQLite (`calendar_attachments`). ICS stores `ATTACH` URIs (`/dav/attachments/{id}`), not BASE64. A CalDAV PUT with inline BINARY is extracted. RFC 8607 managed-attachment POST is not implemented.
- Password recovery emails require SMTP (`DCALCON_SMTP_*`). Port 465 uses implicit TLS; other ports require STARTTLS (plain AUTH is refused). Sends are capped at 20s. Without SMTP, an administrator copies the reset link from the Users page. Recovery mail attempts are listed under Administration → Recovery mail (URLs are never stored).

## SQLite

- One writer at a time. WAL + `busy_timeout` + `SetMaxOpenConns(1)` per process.
- All-in-one compose is the supported production topology (`dcalcon serve`). Split compose is lab/debug only: each process has its own rate limiter, and SQLite still has one writer. Images expose `/healthz` (core) and a login probe (web); sample compose waits for those before starting Caddy.
- The core container drops to uid 1000 (`dcalcon`) after chowning `/data`. Named volumes from older root images are fixed on start.
- Split compose = multiple processes, same file, **one machine**, no replicas.
- Do not put the DB on NFS.
- Backup: `dcalcon backup [path]` (`VACUUM INTO`) or `DCALCON_BACKUP_DIR` for periodic snapshots. Optional `DCALCON_BACKUP_HOOK` receives the snapshot path as argv[1] (copy off-box). Restore: stop the process, `dcalcon restore backup.db`. Restore refuses if the server still holds the runtime lock.

## External providers

- Outlook / Microsoft 365: **no CalDAV**. Graph API only. Connecting an account uses Graph **Mail.Send** (MIME), not calendar sync.
- Gmail: CalDAV+OAuth exists; this product uses **Gmail send** (`gmail.send`) for iMIP, not the Google Calendar API.
- Connecting an account requires developer OAuth clients (Google Cloud + Entra ID) and a redirect URL on **the dashboard origin** (`/api/v1/oauth/{google|microsoft}/callback`).
- Bidirectional sync with Google/Microsoft will create duplicate-UID and organiser-rewrite problems; it is **not** implemented. Treat calendar sync as a dedicated future subsystem.
- Tokens and authenticator secrets are encrypted at rest with AES-GCM (`DCALCON_TOKEN_KEY`, or a key file next to the SQLite database). A full user backup of those fields only restores on a server with the same key.

## go-webdav

- CRUD and calendar-query still go through go-webdav.
- `getctag`, `calendar-color`, RFC 6578 `sync-collection`, and RFC 6638 principal properties are layered in `internal/davext` / `internal/dav` because v0.7.0 does not expose them on the Backend.

## Security

- Terminate TLS in front of the process (Caddy `Caddyfile.tls`). `https://` public URLs force `Secure` session cookies and HSTS.
- First-boot admin password cannot be `changeme` (or other trivial values). Compose requires `DCALCON_ADMIN_PASSWORD`.
- Login, recovery, reset, TOTP reset, and DAV Basic are rate-limited (default 8 failures / 15 minutes, then lockout). Webcal is limited separately (120 fetches / hour / IP).
- Cookie API mutations with an `Origin` header must match `DCALCON_PUBLIC_URL` / `DCALCON_CORS_ORIGINS` (plus localhost:3000). Cross-site fetches that still include the session cookie are rejected. SameSite=Lax still applies.
- `X-Forwarded-For` is trusted only from loopback and `DCALCON_TRUSTED_PROXIES` (rightmost hop). `/metrics` is the TCP peer loopback or `DCALCON_METRICS_TOKEN`; it is not reverse-proxied in the sample Caddyfiles.
- HTTP bodies larger than 10 MiB (user backup upload allows 64 MiB); ICS > 8 MiB; vCard > 1 MiB; PHOTO values > 256 KiB are rejected. Event/task files are stored outside ICS (8 MiB each, 20 per item, 32 MiB total) and served as downloads only. REPORT bodies with `<!DOCTYPE` / `<!ENTITY` are rejected.
- API and DAV responses set `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options`, `Permissions-Policy`, and a restrictive CSP.
- Recovery tokens and session IDs are hashed at rest (`SHA-256`) and recovery tokens expire in two hours; the worker deletes expired sessions, OAuth states, and tokens. The recovery outbox does not keep reset URLs.
- Account → **Revoke other sessions** and **Download your data**. Settings has two restorable zip backups: **data** (calendars, tasks, contacts, files) and **full** (also password, authenticator, app passwords, mail tokens). Full export/import requires the current dashboard password. Admins can read a short audit log.
