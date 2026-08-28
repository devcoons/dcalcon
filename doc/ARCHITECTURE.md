# Architecture

One origin. DAVx⁵ (or GNOME Online Accounts) is added once and sees calendars and contacts. Stored objects are iCalendar and vCard — what clients PUT. The dashboard edits those same rows; it does not keep a parallel event store.

```
                    LAN / internet
                          |
                    reverse proxy
                    (TLS, one host)
                          |
        +--------+--------+--------+--------+
        |        |        |        |        |
     CalDAV   CardDAV    Next    REST    worker
        |        |        |        |        |
        +--------+--------+--------+--------+
                          |
                       SQLite
                       (WAL)
```

Production is `dcalcon serve`: CalDAV, CardDAV, REST, and the worker loop in one process. That is the SQLite-safe default.

Split compose is the same binary with different `CMD`s, one volume, one machine. Lab use. Each process has its own rate limiter. Clients still see one host — do not publish CalDAV and CardDAV on two names.

`/.well-known/caldav` and `carddav` plus `/dav/principals/` live on that origin so one account finds both home-sets.

## Tree

```
src/svc/cmd/dcalcon/          serve | caldav | carddav | api | worker
src/svc/services/             HTTP wrappers for split compose
src/svc/internal/
  storage/                    SQLite, migrations, ACL shares
  caldav/ carddav/ dav/       go-webdav backends + extensions
  api/                        dashboard REST
  worker/                     Important Dates, purge
  schedule/                   local iTIP
  icsutil/                    parse/build ICS and vCard
  userbackup/                 Settings zip
  mail/ providers/ secret/    SMTP, OAuth, sealed blobs
src/cli/                      REST client (no svc internals)
src/web/                      Next.js
```

`dcalcon` is the server entrypoint. Do not fork four copies of the domain into four binaries. `dcalcon-cli` is a client of `/api/v1` only.

Attachments sit in `calendar_attachments` as blobs. ICS carries `ATTACH` URIs, not BASE64. A dashboard edit writes iCalendar/vCard; a DAV PUT overwrites the document. JSCalendar is a JSON mapping for the API, not a second store.

## Auth

| Who | How |
|---|---|
| DAV | HTTP Basic (account password, or app password; app password required once TOTP is on) |
| Web | `dcalcon_session` cookie |
| CLI | same cookie, also `Authorization: Bearer` |

Calendar ACL is the shares table. Dashboard and the CalDAV ACL subset both write it. Not per-event ACEs, not CS:invite.

## Worker

Timer in-process (or the `worker` subcommand in split compose):

- rebuild Important Dates
- expire sessions / OAuth states / reset tokens
- alarms are `VALARM` on the ICS — clients fire them; there is no push service

Invitation mail (iMIP) is sent from the API when someone invites, not from a separate mail daemon.

## Why one database

Principals, passwords, and Important Dates (contacts → calendar) share users. Splitting CalDAV and CardDAV stores would make birthday events a distributed transaction for no gain.
