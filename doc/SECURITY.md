# Threat notes

dCalCon is a single-node calendar/contacts server. The SQLite file is the system of record. Treat it like a password database.

## Trust boundaries

- **Browser dashboard** — cookie session (`dcalcon_session`), SameSite=Lax, Origin allow-list on mutating requests, and `Sec-Fetch-Site: cross-site` is rejected when the session cookie is present.
- **CalDAV/CardDAV** — HTTP Basic. Account password is enough until TOTP is enabled; then only app passwords (`dcc_…`) are accepted.
- **Public webcal** — unauthenticated GET `/webcal/{token}.ics` of **VEVENT** objects on a personal calendar. The token is the secret (36 hex chars). Rotate to revoke. Inbox/outbox/important-dates cannot be published. Fetches are rate-limited per IP. Caddy must proxy `/webcal*` to the Go process (the sample Caddyfiles do); otherwise the Next.js app returns 404.
- **TLS** — terminate in the reverse proxy. Set `DCALCON_PUBLIC_URL` to `https://…` so cookies are Secure and HSTS is sent. HSTS is **not** taken from `X-Forwarded-Proto` (that header is easy to spoof).
- **Client IP** — `X-Forwarded-For` is used only when the TCP peer is loopback or a CIDR in `DCALCON_TRUSTED_PROXIES`. Direct hits on `:8080` ignore XFF. The sample compose files trust RFC1918 so Caddy/Docker can forward the real client; do **not** publish `:8080` on the internet if you set that.
- **Metrics** — `GET /metrics` is loopback-only (TCP peer, not `X-Forwarded-For`) unless you send `Authorization: Bearer` matching `DCALCON_METRICS_TOKEN`. Sample Caddyfiles do **not** proxy it.

## Attachments

Files live in SQLite, not as BASE64 in ICS. Downloads (REST and `/dav/attachments/{uuid}`) always use `Content-Disposition: attachment`, `Cache-Control: private, no-store`, `X-Content-Type-Options: nosniff`, and a `sandbox` CSP. HTML/SVG/JS-looking bytes are served as `application/octet-stream`. Public IDs must be UUIDs. The dashboard must never render these files inline.

## What we already reject

- HTTP bodies larger than 10 MiB (user backup upload 64 MiB); ICS > 8 MiB; vCard > 1 MiB; PHOTO > 256 KiB; attachments 8 MiB each (20 per event/task, 32 MiB total).
- Login, password reset, TOTP reset (`/api/v1/auth/reset-totp`), recovery, and DAV Basic brute force (per IP+user or per IP lockout).
- XML `<!DOCTYPE>` / `<!ENTITY>` on REPORT (no DTD expansion).
- RRULE expansion capped (hundreds of instances) so a hostile series cannot hang calendar-query or free/busy.
- Object hrefs and calendar slugs that contain `/`, `\`, or `..` (zip/ICS filenames are basename-only).
- Recovery outbox never stores or returns the reset URL. Only the admin “send recovery” response includes the link (for when SMTP is down). Public `POST /api/v1/auth/recover` always returns `{"ok": true}`.

## Restore

See [CLIENTS.md](CLIENTS.md) restore drill. `dcalcon restore` replaces the database; it does not merge. It refuses if a dCalCon process still holds the runtime lock. Keep copies off the machine that runs the server (`DCALCON_BACKUP_HOOK` after each snapshot).

Settings also offers a **per-user zip**. A **data** backup restores calendars, tasks, contacts, and files for the signed-in user. A **full** backup also restores the password hash, authenticator, app passwords, and mail tokens — treat that zip like a password file. Mail tokens only decrypt on a server with the same `DCALCON_TOKEN_KEY`. Authenticator secrets in a full backup are also sealed with that key. Full restore refuses a zip that belongs to a different username.

## What this does not cover

- Split compose processes do not share the in-memory rate limiter. Production should run `dcalcon serve` (all-in-one).
- Webcal URLs are as secret as the people you paste them to.
- There is no per-object ACL beyond calendar shares and read-only collections. The CalDAV ACL method rewrites those same whole-calendar shares.
- Logged-in users can query free/busy for local directory users (invite guests). Requests are capped at 20 usernames.
