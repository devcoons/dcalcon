# dCalCon

Self-hosted **CalDAV** + **CardDAV** server (Go) with a **user / admin web app** (Next.js) and **SQLite**. Compatible in intent with GNOME Calendar, GNOME Contacts, DAVx⁵, and other standards-based clients — plus features those stacks usually leave out: an **Important Dates** calendar, **internal event invitations**, and **Gmail / Outlook iMIP** for external guests.

This repository is the **project skeleton**: architecture, protocol choices, limitations, Docker layout, and a compiling core that already stores calendars and contacts.

---

## What we are building

Four product surfaces, one system of record:

| Surface | Role | Stack |
|---|---|---|
| **CalDAV server** | Calendars, events, tasks, scheduling inbox later | Go, RFC 4791 + related specs |
| **CardDAV server** | Address books and contacts | Go, RFC 6352 |
| **User / admin web** | Login, calendars, contacts, invitations, Important Dates, admin users | Next.js 16 |
| **CLI** | Same dashboard REST operations from a terminal | Go (`src/cli`, `dcalcon-cli`) |

Clients never have to use the web UI. The dashboard and CLI talk REST to the same SQLite data that DAV clients sync.

### Extra mechanisms (first-class, not plugins)

1. **Important Dates** — opt-in, server-generated, read-only calendar. Reads `BDAY` / `ANNIVERSARY` from the user’s contacts and publishes yearly all-day events with the alarms the user selected.
2. **Internal invitations** — user A puts user B on an event. B is notified in the dashboard (and later via CalDAV scheduling inbox), accepts or declines, and on accept the event is written to B’s personal calendar.
3. **Connected email (Gmail / Outlook)** — not CalDAV-to-CalDAV. Google and Microsoft are reached with **OAuth + their APIs**, and external attendees get **iMIP** (`text/calendar` email). See [doc/LIMITATIONS.md](doc/LIMITATIONS.md).
4. **User backup** — Settings can download a restorable zip of calendars/contacts/files, or a **full** zip that also restores sign-in secrets (password required). Treat a full zip like a password file.

---

## Feasibility

**Yes, this is a real product** — Baikal, Radicale, DAViCal, Nextcloud, and SOGo prove the category. Doing it *well* (GNOME + DAVx⁵ + Apple + Thunderbird) is a **multi-month** protocol project, not a weekend CRUD app.

What is already realistic in this skeleton:

- One binary `dcalcon` with subcommands `serve | caldav | carddav | api | worker`
- SQLite schema for users, calendars, iCalendar objects, address books, vCards, invitations, Important Dates settings, connected accounts
- HTTP Basic for DAV (account password or an **app password** from Account), cookie sessions for the web API
- RFC 6764 `/.well-known/caldav` and `/.well-known/carddav`
- Principal discovery with both `calendar-home-set` and `addressbook-home-set`
- PUT/GET/DELETE + REPORT calendar-query / addressbook-query via [emersion/go-webdav](https://github.com/emersion/go-webdav)
- Worker that rebuilds Important Dates
- REST for dashboard CRUD + invite/accept/decline
- Multi-target Dockerfile and two compose samples

What is **not** finished (and must not be pretended):

- Two-way Google Calendar API / Microsoft Graph **calendar sync** (iMIP send is implemented; sync is not)
- Full RFC 3744 / CS:invite (whole-calendar shares and a CalDAV ACL subset are implemented)

Plan the work in [doc/ROADMAP.md](doc/ROADMAP.md). Target process/domain diagram: [doc/ARCHITECTURE.md](doc/ARCHITECTURE.md). Protocol list: [doc/STANDARDS.md](doc/STANDARDS.md).

---

## Technical limitations (the ones that shape the design)

### 1. There is no “latest CalDAV 2.0”

CalDAV is still **RFC 4791** (2007) plus **RFC 6638** scheduling (2012). CardDAV is **RFC 6352**. “Latest” means implementing those RFCs **plus** the interoperability extensions clients actually send (sync-collection, ctag, calendar-color, vCard 4, well-known URLs). JSCalendar (RFC 8984) is a JSON model for *new HTTP APIs*, not a replacement for DAV. We store **iCalendar / vCard** as canonical data and can map to JSON for the web.

### 2. SQLite and multiple containers

SQLite is excellent for a **single-node** server (home, team, SMB). It is a poor fit for horizontally scaled writers.

- **Recommended compose:** `sample-all-in-one.docker-compose.yaml` — one Go process (`dcalcon serve`) owns the database (WAL, busy timeout, one writer). This is the **production** layout.
- **Split compose:** `sample-split.docker-compose.yaml` — lab/debug only. CalDAV, CardDAV, API, worker as separate processes sharing a volume. Each process has its own rate limiter. Do **not** replica-scale them. WAL makes this usable on one host; it is not a cluster.

If you later need multiple app nodes, the storage layer is the swap point (Postgres), not the DAV URL layout.

### 3. Gmail and Outlook are not CalDAV peers

- **Google** still exposes CalDAV, but only with **OAuth**. Password CalDAV is gone. For a product, Google **Calendar API** is the supported path.
- **Microsoft 365 / Outlook.com** do **not** speak CalDAV. The supported path is **Microsoft Graph**.

“Connect Gmail/Outlook” means OAuth apps you register, encrypted refresh tokens in SQLite, and **iMIP send** from the connected mailbox. Users of *this* server still use CalDAV on their phones. Two-way calendar sync is not implemented.

### 4. Client quirks

DAVx⁵, GNOME Online Accounts, and Apple Calendar are picky about:

- Trailing-slash redirects (Go’s default `ServeMux` will break DAV — we avoid it)
- `current-user-principal` and home-sets on **one host**
- ETags and `If-Match` / `If-None-Match` on PUT
- REPORT `calendar-query` / `calendar-multiget` / `addressbook-multiget`

GNOME Calendar is CalDAV-only; GNOME Contacts is CardDAV-only. Android needs DAVx⁵ (or similar), not a custom app.

### 5. Scheduling is two stacks

Internal invite/accept is REST plus CalDAV inbox objects and ORGANIZER/ATTENDEE on the stored VEVENT. Apple “invite this person” still needs more of RFC 6638 (outbox POST). External people get **iMIP** once a Gmail, Microsoft, or SMTP account is linked in Settings.

---

## Repository layout

Three products:

| Path | What it is |
|---|---|
| **`src/svc`** | The Go **service**: CalDAV + CardDAV + REST + worker. One binary (`dcalcon`) with subcommands `serve \| caldav \| carddav \| api \| worker`. |
| **`src/web`** | The Next.js **dashboard**. Talks REST to the service; never talks SQLite. |
| **`src/cli`** | The Go **CLI**. Same `/api/v1` as the dashboard; never imports service internals. Binary `dcalcon-cli`. |

```
doc/                  Architecture, roadmap, clients, standards
src/svc/              Go module (server)
  cmd/dcalcon/        Unified binary
  services/           Thin HTTP wrappers for split compose
  internal/           Storage, auth, DAV backends, API, worker
  caddy/              Sample reverse-proxy configs
  configs/            Example YAML
src/cli/              Go module (REST client)
  cmd/dcalcon-cli/    Dashboard CLI
src/web/              Next.js dashboard (App Router)
Dockerfile            Multi-target: core, caldav, carddav, api, worker, web
sample-*.docker-compose.yaml
README.md
```

Public URL map (one origin, required for client discovery):

| Path | Service |
|---|---|
| `/.well-known/caldav` | 301 → `/dav/` |
| `/.well-known/carddav` | 301 → `/dav/` |
| `/dav/principals/{user}/` | principal |
| `/dav/calendars/{user}/{calendar}/` | CalDAV collection |
| `/dav/addressbooks/{user}/{book}/` | CardDAV collection |
| `/api/v1/...` | web / admin JSON |
| `/webcal/{token}.ics` | public calendar feed (Caddy must proxy this to core) |
| `/` | Next.js UI |

DAV authentication: **HTTP Basic** (dashboard password or an app password). Web and CLI: **session cookie** (CLI also sends Bearer). Liveness `/healthz`, readiness `/readyz` (SQLite ping). Prometheus text `/metrics` is loopback-only (or `DCALCON_METRICS_TOKEN`).

---

## Run locally

Needs Go 1.26+ and Node 22+.

```bash
export PATH="/usr/lib/golang/bin:$PATH"   # if go is not on PATH
mkdir -p data
cd src/svc
DCALCON_SQLITE_PATH=../../data/dcalcon.db \
DCALCON_HTTP_ADDR=:8080 \
DCALCON_ADMIN_USERNAME=admin \
DCALCON_ADMIN_PASSWORD=dcalcon-dev-pass \
go run ./cmd/dcalcon serve
```

Web UI (another terminal):

```bash
cd src/web && npm install && npm run dev
```

Open `http://localhost:3000`, sign in as `admin` / `dcalcon-dev-pass`. Point DAVx⁵ at `http://localhost:8080` with an app password from Account (or the same password).

CLI (another terminal; talks to the same API as the dashboard):

```bash
make -C src/cli build
./bin/dcalcon-cli --url http://127.0.0.1:8080 login --user admin
./bin/dcalcon-cli calendar list
./bin/dcalcon-cli help
```

Session is stored in `~/.config/dcalcon/cli.json` (mode `0600`). Override with `--url`, `--config`, `DCALCON_URL`, `DCALCON_CLI_CONFIG`, or `DCALCON_SESSION`. Use `--json` for scripting.

```bash
make -C src/svc test
make -C src/cli test
cd src/web && npm test && npm run typecheck
```

---

## Run with containers

All-in-one (production with SQLite):

```bash
cp .env.example .env
podman compose -f sample-all-in-one.docker-compose.yaml up --build
# or: docker compose -f sample-all-in-one.docker-compose.yaml up --build
```

Then `http://localhost` (Caddy on port 80). Set `DCALCON_ADMIN_PASSWORD` in `.env` first (not `changeme`).

For HTTPS, mount `src/svc/caddy/Caddyfile.tls`, publish 80+443, and set `DCALCON_PUBLIC_URL=https://your.domain`.

Backup / restore:

```bash
dcalcon backup                 # VACUUM INTO data/backups/dcalcon-….db
dcalcon restore path/to.db     # stop the server first; refused if still running
```

Set `DCALCON_BACKUP_HOOK` to a program that copies argv[1] off the machine. Practice the restore once before you need it: [doc/CLIENTS.md](doc/CLIENTS.md) (restore drill) and [doc/SECURITY.md](doc/SECURITY.md).

Split processes (lab/debug only — still one node, shared volume; not a production topology):

```bash
podman compose -f sample-split.docker-compose.yaml up --build
```

Change the bootstrap password. The first empty database creates the admin from `DCALCON_ADMIN_*` (`changeme` is rejected).

---

## Default objects per user

On user create:

- Calendar `personal` (read-write)
- Address book `contacts`
- Important Dates settings (off until the user enables them)

The worker then creates calendar `important-dates` when that setting is on.

---

## Configuration

| Variable | Meaning |
|---|---|
| `DCALCON_CONFIG` | Optional YAML (see `src/svc/configs/dcalcon.example.yaml`) |
| `DCALCON_HTTP_ADDR` | Listen address (`:8080`) |
| `DCALCON_PUBLIC_URL` | Public origin |
| `DCALCON_SQLITE_PATH` | Database file |
| `DCALCON_ADMIN_USERNAME` / `DCALCON_ADMIN_PASSWORD` | First-run admin (password cannot be `changeme`) |
| `DCALCON_SESSION_SECURE` | Set `true` behind HTTPS (automatic when `DCALCON_PUBLIC_URL` is `https://`) |
| `DCALCON_CORS_ORIGINS` | Extra allowed dashboard origins (comma-separated) |
| `DCALCON_BACKUP_DIR` | Periodic SQLite snapshots (`VACUUM INTO`) |
| `DCALCON_BACKUP_HOOK` | Optional program run after each snapshot with the file path as argv[1] |
| `DCALCON_AUTH_MAX_ATTEMPTS` / `DCALCON_AUTH_WINDOW` / `DCALCON_AUTH_LOCKOUT` | Login and DAV Basic lockout |
| `DCALCON_TOKEN_KEY` | AES-GCM key for OAuth/SMTP secrets (hex or passphrase; auto-file if empty) |
| `DCALCON_SMTP_HOST` / `PORT` / `USERNAME` / `PASSWORD` / `FROM` | Optional mail for password recovery (without it, copy a link from Users) and iMIP fallback |
| `GOOGLE_OAUTH_*` / `MICROSOFT_OAUTH_*` | iMIP send (Gmail / Graph). Redirect URI is `{origin}/api/v1/oauth/{provider}/callback` |

---

## License

Not chosen yet. Treat this as an internal project until you add one.
