# Limitations

## Product

- Recurring events keep RRULE. calendar-query time-range matches instances; `<expand/>` rewrites `calendar-data` (capped). The dashboard edits FREQ/INTERVAL/UNTIL/COUNT.
- Each user has a Schedule Inbox and Outbox. REST invites and CalDAV PUTs with an `ATTENDEE` that maps to a local user drop `METHOD:REQUEST` in that inbox. Outbox POST, free/busy REPORT, CANCEL on delete, REPLY PARTSTAT merge are in. `calendar-auto-schedule` is advertised. iMIP is send-only. `@dcalcon.private` is never mailed.
- Whole-calendar share with another local user: dashboard plus CalDAV ACL/privilege props on the same rows. Shared collections show up as `x-share-{id}`. No per-object ACE, no CS:invite.
- Personal calendars advertise VEVENT and VTODO. VJOURNAL is not advertised; PUT is 403.
- TOTP is a second factor on the dashboard password. With it on, DAV only accepts an app password.
- Files live in SQLite. ICS has `ATTACH` URIs (`/dav/attachments/{id}`). A PUT with inline BINARY is pulled out. RFC 8607 managed-attachment POST is not implemented.
- Recovery mail needs `DCALCON_SMTP_*`. Port 465 is implicit TLS; anything else must STARTTLS (plain AUTH is refused). 20s send cap. No SMTP: copy the link from Users. Administration → Recovery mail lists attempts, not URLs.

Run `go test ./internal/dav/` and actually sync DAVx⁵ / GNOME over HTTPS before calling it production.

## SQLite

- One writer. WAL, `busy_timeout`, `SetMaxOpenConns(1)` per process.
- All-in-one compose (`dcalcon serve`) is the supported layout. Split compose is lab-only; healthchecks are `/healthz` on core and a login probe on web.
- Core image drops to uid 1000 after chowning `/data`. Old root-owned volumes get fixed on start.
- Do not put the DB on NFS.
- `dcalcon backup [path]` (`VACUUM INTO`) or `DCALCON_BACKUP_DIR`. Optional `DCALCON_BACKUP_HOOK` argv[1] = snapshot. Restore: stop the process, `dcalcon restore backup.db`. Restore bails if the runtime lock is held.

## External providers

- Microsoft 365 / Outlook.com: Graph Mail.Send. No CalDAV.
- Gmail: `gmail.send` for iMIP, not the Calendar API.
- OAuth clients go in Google Cloud / Entra ID. Redirect is on the dashboard origin: `/api/v1/oauth/{google|microsoft}/callback`.
- Bidirectional calendar sync would duplicate UIDs and rewrite organizers. Not implemented.
- Tokens and TOTP secrets are AES-GCM (`DCALCON_TOKEN_KEY`, or a key file next to the DB). A full user zip only decrypts those fields on a server with the same key.

## go-webdav

CRUD and calendar-query still go through it. `getctag`, `calendar-color`, RFC 6578 `sync-collection`, and RFC 6638 principal properties are in `internal/davext` / `internal/dav` because v0.7.0 does not expose them on the Backend.

## Security (short)

TLS in front (`Caddyfile.tls`). `https://` public URL → Secure cookies and HSTS. First-boot password cannot be `changeme`. Login, recovery, reset, TOTP reset, and DAV Basic are rate-limited (default 8 / 15m then lockout). Webcal is 120 fetches / hour / IP.

Cookie mutations with an `Origin` header must match `DCALCON_PUBLIC_URL` / `DCALCON_CORS_ORIGINS` (and localhost:3000). `X-Forwarded-For` only from loopback or `DCALCON_TRUSTED_PROXIES` (rightmost hop). `/metrics` is the TCP peer (loopback) or `DCALCON_METRICS_TOKEN`.

Size caps: 10 MiB HTTP (64 MiB user-backup upload), 8 MiB ICS, 1 MiB vCard, 256 KiB PHOTO, attachments 8 MiB × 20 / 32 MiB total. REPORT with `<!DOCTYPE` / `<!ENTITY` is dropped.

Sessions and recovery tokens are SHA-256 at rest; recovery tokens last two hours. Worker deletes expired sessions, OAuth states, and tokens. Settings zip: **data** = calendars/tasks/contacts/files; **full** also has password, authenticator, app passwords, mail tokens — same as handing someone a password file. Full import asks for the current dashboard password.
