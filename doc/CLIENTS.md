# Client notes

All clients should use the **same origin** as the web UI (or a dedicated `dav.` host that still serves both well-known URLs).

Username / password = dCalCon user (HTTP Basic). Prefer an **app password** created under Account.

## Inviting other people on this server

CalDAV apps (DAVx⁵, Thunderbird, Apple Calendar) invite guests by **email**, not by dCalCon username.

Each account has a reserved address: **`username@dcalcon.private`**.

1. Sync CardDAV and enable the read-only address book **People on this server**.
2. Those cards already use `username@dcalcon.private` as EMAIL.
3. When you add that person as an attendee, dCalCon delivers a local invitation (Inbox), not internet mail.

You can also add the same EMAIL by hand to your personal Contacts. Never send SMTP to `@dcalcon.private` — the server treats it as local only. Override the domain with `DCALCON_SCHEDULING_DOMAIN` if you want a different suffix.

## DAVx⁵ (Android)

1. Add account → **Generic CalDAV/CardDAV** (or “DAVx⁵”).
2. Base URL: `https://your.example.com/` (well-known must work) or `https://your.example.com/dav/`.
3. Username = dCalCon user. Password = an **app password** from Account. If the account has an authenticator enabled, the dashboard password is rejected for DAV — app passwords only.
4. Enable the Personal calendar, Important Dates (read-only), and Contacts.
5. Tasks: VTODO is advertised; install OpenTasks / tasks.org if you want them.

## GNOME Calendar / Contacts

GNOME Online Accounts → **Calendar, Contacts and Tasks** (or separate CalDAV / CardDAV if your distro still splits them).

Server: `https://your.example.com/`

GNOME Calendar does not replace Contacts; enable both.

## Thunderbird

Calendars → New → On the network → CalDAV URL:

`https://your.example.com/dav/calendars/{username}/personal/`

Address book → CardDAV:

`https://your.example.com/dav/addressbooks/{username}/contacts/`

## Apple Calendar / Contacts

Works if discovery, principals, ETags, If-Match, getctag, and calendar-color are correct. Inbox/outbox URLs exist; outbox POST, free/busy REPORT, and `calendar-auto-schedule` are advertised. Shared dashboard calendars appear in the calendar home as `x-share-{id}` and on `calendar-proxy-read-for` / `calendar-proxy-write-for`. Clients that send MKCALENDAR (displayname / calendar-color) get those properties stored. VJOURNAL is not supported. ACL on a calendar collection maps to the same whole-calendar shares as the dashboard.

## Restore drill

Keep at least two generations **off the machine** that runs SQLite. Periodic files in `DCALCON_BACKUP_DIR` live on the same volume as the database unless you copy them.

Off-box copy: set `DCALCON_BACKUP_HOOK` to a program. After each snapshot the server runs `hook <snapshot-path>`. Example script:

```sh
#!/bin/sh
# /usr/local/bin/dcalcon-offbox
exec rsync -a "$1" backup-host:/var/backups/dcalcon/
```

Then:

```sh
export DCALCON_BACKUP_HOOK=/usr/local/bin/dcalcon-offbox
```

Practice once:

1. `dcalcon backup` (or wait for the periodic snapshot). Confirm the hook copied the file off-box.
2. Stop the process (compose: `docker compose stop core`). Restore is refused while `dcalcon serve` (or a split process) still holds the lock.
3. `dcalcon restore path/to.db` — this replaces the live SQLite file and keeps `*.pre-restore`.
4. Start the process. Sign in, open Calendars and Contacts, and sync one DAV client.

A restore is not a merge; it is a full replace.

## Interop gate

Before a production cut:

1. `go test ./internal/dav/ ./internal/icsutil/` (ctag, sync-collection, If-Match, expand, CardDAV query).
2. DAVx⁵: add generic CalDAV/CardDAV, sync Personal + Contacts, create an event on the phone, confirm it in the dashboard.
3. GNOME Calendar + Contacts: add the same origin, round-trip one event and one contact.

## Outlook desktop / Microsoft 365

Will **not** add this server as CalDAV. Users should use DAVx⁵ on Android, GNOME, Thunderbird, or Apple — or a future Graph bridge.
