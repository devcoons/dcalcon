# Architecture

## Goals

- One host that DAVx⁵ / GNOME Online Accounts can add **once** and get calendars **and** contacts.
- iCalendar and vCard remain the canonical payload (what clients PUT).
- The web UI is a first-class editor of the same objects, not a second database.
- Extra features (Important Dates, invites, OAuth) run as jobs against the **same domain layer** as DAV.

## Target shape

This is the diagram we should grow into. It matches the usual “reverse proxy + protocol adapters + domain + SQLite + worker” split, with two corrections:

1. The **worker is a client of the domain**, not a second writer glued to the database.
2. Next.js and the Go REST API are **two processes** behind the proxy, even if they are one “web” product.

```
                         Internet / LAN
                               |
                        +---------------+
                        | Reverse proxy |
                        | TLS + routing |
                        | one public origin
                        +-------+-------+
                                |
     +----------+----------+----+-----+----------+
     |          |          |          |          |
+----v---+ +----v----+ +---v--+ +-----v----+ +---v----+
| CalDAV | | CardDAV | | Web  | | API/REST | | Worker |
| Go     | | Go      | | Next | | Go       | | Go     |
+----+---+ +----+----+ +--+---+ +-----+----+ +---+----+
     |          |         |           |          |
     +----------+---------+-----------+----------+
                                |
                     +----------v-----------+
                     | Shared domain        |
                     | Users / ACL          |
                     | Calendars / Contacts |
                     | Scheduling (iTIP)    |
                     | Notification policy  |
                     | Integration ports    |
                     +----------+-----------+
                                |
                          +-----v-----+
                          |  SQLite   |
                          | WAL mode  |
                          +-----------+
```

**All-in-one (`dcalcon serve`)** — one process: CalDAV + CardDAV + REST + worker goroutine. This is the SQLite-safe **production** default.

**Split** — same binary, different `CMD`. Shared volume, **one node**, no replicas. Lab/debug only: rate limits are per-process. Caddy still exposes **one origin**. Clients must not see two hosts for CalDAV vs CardDAV.

Discovery (`/.well-known/*`, `/dav/principals/`) is not a fourth product. It is shared routing on that origin so one DAVx⁵ account finds both home-sets.

## Domain vs adapters

| Layer | Owns | Must not own |
|---|---|---|
| CalDAV / CardDAV | RFC 4791 / 6352 HTTP, REPORT XML, ETags | Password hashing policy, invite business rules |
| API | JSON for the dashboard | A second event model that DAV cannot round-trip |
| Web (Next) | UI | Direct SQLite |
| Worker | Important Dates generation, iMIP send, OAuth sync ticks | Ad-hoc SQL that bypasses ACL |
| Domain | Users, ACL, calendars, contacts, iTIP scheduling, notification prefs | HTTP, XML, OAuth HTTP details |
| Store | SQLite, WAL, migrations | “What is a birthday calendar” |

Canonical bytes: **raw iCalendar / vCard** plus extracted columns for queries. File attachments for events and tasks are **SQLite blobs** (`calendar_attachments`); ICS holds `ATTACH` URIs, not the file bytes. DAV PUT overwrites the document. The API generates iCalendar/vCard when the user edits in the dashboard. JSCalendar (RFC 8984) is a **mapping for JSON**, not a second store.

## Target Go layout

Keep the unified binary. Grow internals toward this (names can land incrementally):

```
src/svc/                     Go module (one server binary, several CMDs)
  cmd/dcalcon/               serve | caldav | carddav | api | worker
  services/{caldav,carddav,api,worker}/
  internal/
    domain/                  users, ACL, calendars, contacts, settings
    store/                   SQLite adapter (today: internal/storage)
    auth/
    ical/                    parse / validate / encode RFC 5545
    vcard/                   parse / validate / encode RFC 6350 + v3
    schedule/                iTIP state machine (RFC 5546 → 6638 + 6047)
    integrations/            Google, Microsoft, SMTP
  caddy/                     sample reverse-proxy configs
src/cli/                     Go module (REST client of /api/v1)
  cmd/dcalcon-cli/           dashboard operations from the terminal
  internal/{config,client,app}/
src/web/                     Next.js dashboard
```

`src/svc/cmd/dcalcon` is the only **server** Go entrypoint. Do not split the service into four unrelated binaries with copied domain code. `dcalcon-cli` is a separate client of the dashboard REST API — it must not import `src/svc/internal/*`.

## Auth

| Client | Method |
|---|---|
| DAV | HTTP Basic, bcrypt password |
| Web | `dcalcon_session` cookie (and optional Bearer) |
| CLI | Login stores `dcalcon_session`; requests send the cookie and `Authorization: Bearer` |

App passwords can be added later without changing collection URLs. Calendar ACL (RFC 3744 subset) is enforced in storage shares; DAV ACL and the dashboard both write that table.

## Worker jobs

The worker calls domain use-cases on a timer (and later on a jobs table):

- Important Dates rebuild
- Alarm materialization into `VALARM` (clients display/notify; we do not run a push-notification platform on day one)
- Invitation delivery (local inbox; iMIP to external addresses when a mail account is linked)
- Google / Microsoft **calendar** sync (not started; mail send is in the API, not the worker)

Alarms: persist policy in domain, emit standard `VALARM` on the ICS. Do not invent a parallel reminder channel unless DAVx⁵ / GNOME are proven insufficient.

## Why not two databases

CalDAV and CardDAV share principals, passwords, and Important Dates (contacts → calendar). Splitting storage would force a distributed transaction for a feature that is the product’s point.
