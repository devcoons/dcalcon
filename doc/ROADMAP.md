# Roadmap

## In

- CalDAV/CardDAV CRUD (go-webdav), well-known URLs, principals
- App passwords, TOTP, disable user, admin password reset, recovery mail
- `getctag`, sync-collection, calendar-color, MKCALENDAR props, RRULE expand
- VEVENT + VTODO advertised; VJOURNAL PUT rejected
- Whole-calendar shares (dashboard + CalDAV ACL subset)
- Schedule inbox/outbox, local ORGANIZER/ATTENDEE, outbox POST, free/busy
- People-on-this-server address book (`username@dcalcon.private`)
- Month/week/list dashboard, event/contact editors, CLI
- iMIP via Gmail / Graph / SMTP (send only)
- SQLite backup/restore, per-user data/full zip, rate limits, REPORT entity reject
- Webcal on personal calendars (VEVENT only; token hashed at rest)

## Not done

- Timezone picker in the UI
- vCard 4 round-trip that keeps unknown properties (and KIND=group if GNOME wants it)
- Per-contact Important Dates opt-out, `X-ABDATE` / extra vCard dates
- Drag-drop on the calendar grid
- Two-way Google Calendar API / Graph calendar sync (explicit UX, conflict rules — not started)
- Full RFC 3744 (per-object ACE) and CS:invite
- Sitting down with DAVx⁵ / GNOME / Thunderbird on a live HTTPS box and filing what still breaks

Interop checklist lives in [CLIENTS.md](CLIENTS.md). Spec list: [STANDARDS.md](STANDARDS.md).
