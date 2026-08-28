# Roadmap

## Phase 0 — this repository

- Service split, Docker, SQLite schema, discovery URLs
- Basic CalDAV / CardDAV CRUD through go-webdav
- Web login, calendars, contacts, invitations, Important Dates toggle
- Worker generating Important Dates

## Phase 1 — domain layer and tenancy

- Extract `internal/domain` + `internal/store` so DAV, API, and worker share one use-case layer
- Split `icsutil` into `internal/ical` and `internal/vcard` (unknown properties round-trip)
- HTTPS-only samples, secure cookies — **in** (`Caddyfile.tls`; https public URL forces Secure)
- App passwords for DAV — **in**
- Disable users, password reset by admin — **in**
- Structured logging and request IDs — **in**

## Phase 2 — CalDAV interop

- calendar-color, displayname PROPPATCH — **in**
- getctag + RFC 6578 sync-collection — **in** (change log; unknown tokens force a full resync)
- RRULE expansion for calendar-query time-range and `<expand/>` — **in**
- VTODO / VJOURNAL collection flags that match client expectations — **in** (VEVENT+VTODO advertised; VJOURNAL PUT rejected)
- MKCALENDAR with properties — **in** (displayname, description, calendar-color)
- RFC 3744 subset — **in** (privilege-set / acl on calendars; ACL method maps to whole-calendar shares). Not CS:invite or per-object ACE.
- Interop runs: DAVx⁵, GNOME Calendar, Thunderbird

## Phase 3 — CardDAV interop

- vCard 4 round-trip without dropping unknown properties
- Groups / KIND=group if GNOME needs them
- PHOTO size limits — **in** (256 KiB)
- Interop: GNOME Contacts, DAVx⁵, Thunderbird
- getctag + sync-collection on address books — **in**

## Phase 4 — dashboard completeness

- Month/week views — **in** (month grid; week hour grid with overlap lanes; list). No drag-drop.
- Inline event and contact editors — **in** (create/edit/delete; no grid)
- CLI covering the dashboard REST API — **in** (`src/cli`, `dcalcon-cli`)
- Timezone picker
- Empty / error states against a live core
- Forgot-password + admin recovery link — **in** (SMTP optional)

## Phase 5 — internal scheduling (RFC 6638)

- schedule-inbox-URL / schedule-outbox-URL — **in**
- ORGANIZER / ATTENDEE on VEVENT — **in** for local invites, attendee PUTs, and `username@dcalcon.private`
- CardDAV **People on this server** address book — **in**
- Free/busy (VFREEBUSY) if clients ask — **in** (REPORT + outbox POST + dashboard)
- Outbox POST of iTIP messages — **in**
- Web invitations stay as a view of the same inbox — **in** (accept picker + inbox cleanup)

## Phase 6 — Important Dates polish

- Per-contact opt-out
- Custom dates from `X-ABDATE` / typed vCard dates
- Notification previews in the UI
- Do not clobber client-local copies: keep calendar read-only

## Phase 7 — Gmail / Outlook

- OAuth apps, encrypted tokens — **in** (AES-GCM; Gmail send + Graph Mail.Send + per-user SMTP)
- iMIP send via Gmail API / Graph / SMTP — **in** (send-only invitations, not calendar sync)
- Optional one-way or two-way event sync (explicit UX, conflict policy) — **not** started
- Never pretend Microsoft speaks CalDAV

## Phase 8 — hardening

- Shared calendars via dashboard shares — **in**; CalDAV ACL method + privilege properties map to those shares (RFC 3744 subset, not CS:invite)
- SQLite backup (`dcalcon backup` / `DCALCON_BACKUP_DIR`) plus optional `DCALCON_BACKUP_HOOK` off-box copy; restore refuses a live process — **in**
- Per-user Settings backup zip (data vs full account restore) — **in**
- Login / DAV rate limits, If-Match, size limits, `/readyz` — **in**
- Load tests, fuzz REPORT XML — **in** (REPORT entity reject + calendar-query loop)
- Threat model: path traversal on hrefs, remaining ICS bombs, entity expansion — **in** ([doc/SECURITY.md](SECURITY.md))
