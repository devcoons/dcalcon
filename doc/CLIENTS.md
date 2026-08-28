# Clients

Use the same origin as the web UI (or a `dav.` host that still serves both well-known URLs).

Username is the dCalCon user. Password should be an **app password** from Account. If TOTP is on, the dashboard password is rejected for DAV.

## Inviting people on this server

CalDAV apps invite by email, not by username. Each account has `username@dcalcon.private`.

1. Sync CardDAV and enable the read-only book **People on this server**.
2. Those cards already have `username@dcalcon.private` as EMAIL.
3. Adding that attendee delivers a local invitation (Inbox), not internet mail.

You can type the same EMAIL on a personal contact. Never SMTP to `@dcalcon.private`. Override the suffix with `DCALCON_SCHEDULING_DOMAIN` if you want.

## DAVx⁵ (Android)

Generic CalDAV/CardDAV. Base URL `https://your.example.com/` (well-known) or `https://your.example.com/dav/`. Enable Personal, Important Dates (read-only), Contacts. VTODO is advertised — OpenTasks / tasks.org if you want tasks.

## GNOME

Online Accounts → Calendar, Contacts and Tasks (or the split CalDAV/CardDAV entries some distros still have). Server: `https://your.example.com/`. Calendar does not pull contacts; enable both.

## Thunderbird

CalDAV: `https://your.example.com/dav/calendars/{username}/personal/`

CardDAV: `https://your.example.com/dav/addressbooks/{username}/contacts/`

## Apple Calendar / Contacts

Needs discovery, principals, ETags, If-Match, getctag, calendar-color. Inbox/outbox URLs exist; outbox POST, free/busy, `calendar-auto-schedule` are advertised. Shared calendars appear as `x-share-{id}` and on calendar-proxy-read/write-for. MKCALENDAR with displayname / color is stored. No VJOURNAL. ACL on a collection is the same whole-calendar shares as the dashboard.

## Restore drill

Keep at least two generations off the machine that holds SQLite. Files in `DCALCON_BACKUP_DIR` sit on the same volume unless you copy them.

Hook example:

```sh
#!/bin/sh
# /usr/local/bin/dcalcon-offbox
exec rsync -a "$1" backup-host:/var/backups/dcalcon/
```

```sh
export DCALCON_BACKUP_HOOK=/usr/local/bin/dcalcon-offbox
```

1. `dcalcon backup` (or wait for the periodic snapshot). Check the hook copied it.
2. Stop the process (`docker compose stop core`). Restore is refused while `serve` still holds the lock.
3. `dcalcon restore path/to.db` — replaces the live file, keeps `*.pre-restore`.
4. Start it. Sign in, open Calendars and Contacts, sync one DAV client.

Not a merge.

## Before you call it production

1. `go test ./internal/dav/ ./internal/icsutil/`
2. DAVx⁵: add the account, sync Personal + Contacts, create an event on the phone, see it in the dashboard.
3. GNOME Calendar + Contacts: same origin, one event and one contact round-trip.

## Outlook / Microsoft 365

Will not add this server as CalDAV. Use DAVx⁵, GNOME, Thunderbird, or Apple — or wait for a Graph bridge that does not exist yet.
