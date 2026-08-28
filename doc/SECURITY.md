# Security

Single-node calendar/contacts server. Treat the SQLite file like a password database.

## Boundaries

- **Dashboard** — cookie `dcalcon_session`, SameSite=Lax. Mutating requests with `Origin` must be on the allow-list. `Sec-Fetch-Site: cross-site` with a session cookie is rejected.
- **CalDAV/CardDAV** — HTTP Basic. Account password until TOTP is enabled; then only app passwords (`dcc_…`).
- **Webcal** — unauthenticated GET `/webcal/{token}.ics` of VEVENT on a *personal* calendar. The URL token is 36 hex chars; that is the secret. It is stored hashed (SHA-256). Rotate issues a new URL (shown once). GET afterwards only says the feed is on — copy a new link by rotating. Inbox / outbox / important-dates cannot be published. Rate-limited per IP. Caddy must proxy `/webcal*` to Go (the sample files do); otherwise Next 404s.
- **TLS** — terminate at the proxy. `DCALCON_PUBLIC_URL=https://…` turns on Secure cookies and HSTS. HSTS is not taken from `X-Forwarded-Proto` (spoofable).
- **Client IP** — XFF only if the TCP peer is loopback or in `DCALCON_TRUSTED_PROXIES`. Hitting `:8080` directly ignores XFF. Sample compose trusts RFC1918 so Caddy can pass the real client; do not publish `:8080` on the internet if you set that.
- **Metrics** — `GET /metrics` is loopback by TCP peer, not XFF, unless `Authorization: Bearer` matches `DCALCON_METRICS_TOKEN`. Sample Caddyfiles leave it unproxied.

## Attachments

Blobs in SQLite, not BASE64 in ICS. Downloads (REST and `/dav/attachments/{uuid}`) use `Content-Disposition: attachment`, `Cache-Control: private, no-store`, `nosniff`, sandbox CSP. HTML/SVG/JS-looking bytes go out as `application/octet-stream`. Public IDs are UUIDs. Do not render these inline in the dashboard.

## Rejected

- Bodies > 10 MiB (backup upload 64 MiB); ICS > 8 MiB; vCard > 1 MiB; PHOTO > 256 KiB; attachments 8 MiB each (20 per item, 32 MiB total)
- Login, reset, TOTP reset, recovery, DAV Basic brute force
- XML `<!DOCTYPE>` / `<!ENTITY>` on REPORT
- RRULE expansion capped so a nasty series cannot hang calendar-query or free/busy
- Object hrefs / slugs with `/`, `\`, `..`
- Recovery outbox never stores the reset URL. Only the admin “send recovery” response includes the link (SMTP down). Public `POST /api/v1/auth/recover` always returns `{"ok": true}`

## Restore

See the drill in [CLIENTS.md](CLIENTS.md). `dcalcon restore` replaces the database; it does not merge. Refused while a process holds the runtime lock. Copy snapshots off-box (`DCALCON_BACKUP_HOOK`).

Per-user zip from Settings: **data** = calendars, tasks, contacts, files. **full** also restores password hash, authenticator, app passwords, mail tokens. Mail tokens and TOTP secrets only decrypt with the same `DCALCON_TOKEN_KEY`. Full restore refuses a zip for a different username.

## Not covered

- Split compose does not share the in-memory rate limiter. Run `dcalcon serve`.
- Anyone with a webcal URL can read that calendar.
- No per-object ACL beyond calendar shares and read-only collections.
- Logged-in users can query free/busy for directory users (invite guests), max 20 names.
