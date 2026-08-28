# dCalCon

Self-hosted CalDAV and CardDAV with a Next.js dashboard and SQLite. Phones and desktop apps talk DAV; the web UI and `dcalcon-cli` talk REST. Same database.

Aimed at DAVx⁵, GNOME Calendar/Contacts, Thunderbird, and Apple Calendar. Also: Important Dates (birthdays from contacts), invites between local users, and iMIP through Gmail / Microsoft / SMTP for people who are not on this server.

Google and Microsoft are mail send only. There is no two-way calendar sync with them. Outlook is not CalDAV; do not try to add this server there.

More detail: [doc/ARCHITECTURE.md](doc/ARCHITECTURE.md), [doc/LIMITATIONS.md](doc/LIMITATIONS.md), [doc/CLIENTS.md](doc/CLIENTS.md), [doc/SECURITY.md](doc/SECURITY.md), [doc/ROADMAP.md](doc/ROADMAP.md), [doc/STANDARDS.md](doc/STANDARDS.md).

## Run (dev)

Go 1.26+ and Node 22+.

```bash
mkdir -p data
cd src/svc
DCALCON_SQLITE_PATH=../../data/dcalcon.db \
DCALCON_HTTP_ADDR=:8080 \
DCALCON_ADMIN_USERNAME=admin \
DCALCON_ADMIN_PASSWORD=dcalcon-dev-pass \
go run ./cmd/dcalcon serve
```

```bash
cd src/web && npm install && npm run dev
```

Dashboard is `http://localhost:3000` (`admin` / `dcalcon-dev-pass`). Point a DAV client at `http://localhost:8080`. Prefer an app password from Account; if TOTP is on, DAV will not accept the dashboard password.

```bash
make -C src/cli build
./bin/dcalcon-cli --url http://127.0.0.1:8080 login --user admin
./bin/dcalcon-cli calendar list
```

CLI session file is `~/.config/dcalcon/cli.json` (`0600`). `--url`, `--config`, `DCALCON_URL`, `DCALCON_CLI_CONFIG`, `DCALCON_SESSION` override it. `--json` for scripts.

```bash
make -C src/svc test
make -C src/cli test
cd src/web && npm test && npm run typecheck
```

A new user gets calendar `personal`, address book `contacts`, and Important Dates settings (off). The worker creates `important-dates` when that is turned on.

## Containers

All-in-one is what you should run with SQLite — one `dcalcon serve` process owns the file:

```bash
cp .env.example .env   # set DCALCON_ADMIN_PASSWORD (not changeme)
podman compose -f sample-all-in-one.docker-compose.yaml up --build
```

UI is `http://localhost` (Caddy :80). For TLS, use `src/svc/caddy/Caddyfile.tls`, publish 80/443, set `DCALCON_PUBLIC_URL=https://your.domain`.

`sample-split.docker-compose.yaml` is for poking at individual processes. Same volume, one machine, no replicas. Rate limits are per process. Do not scale it.

```bash
dcalcon backup                 # VACUUM INTO data/backups/dcalcon-….db
dcalcon restore path/to.db     # server must be stopped
```

`DCALCON_BACKUP_HOOK` is a program that gets the snapshot path as argv[1]. Practice restore once: [doc/CLIENTS.md](doc/CLIENTS.md).

## Layout

| Path | |
|---|---|
| `src/svc` | Server. Binary `dcalcon`: `serve`, `caldav`, `carddav`, `api`, `worker` |
| `src/web` | Dashboard. REST only, no SQLite |
| `src/cli` | `dcalcon-cli`. Same `/api/v1` as the dashboard; does not import `src/svc/internal/*` |

```
doc/
src/svc/cmd/dcalcon/     unified binary
src/svc/services/        thin wrappers for split compose
src/svc/internal/        storage, DAV, REST, worker
src/svc/caddy/           sample Caddyfiles
src/cli/cmd/dcalcon-cli/
src/web/
Dockerfile               targets: core, caldav, carddav, api, worker, web
sample-*.docker-compose.yaml
```

One public origin (required for `/.well-known` discovery):

| Path | |
|---|---|
| `/.well-known/caldav`, `/.well-known/carddav` | 301 → `/dav/` |
| `/dav/principals/{user}/` | principal |
| `/dav/calendars/{user}/{calendar}/` | CalDAV |
| `/dav/addressbooks/{user}/{book}/` | CardDAV |
| `/api/v1/...` | dashboard / CLI |
| `/webcal/{token}.ics` | public feed — Caddy must send this to Go, not Next |
| `/` | Next.js |

DAV: HTTP Basic. Web: `dcalcon_session` cookie. CLI sends the cookie and Bearer. `/healthz`, `/readyz` (SQLite ping). `/metrics` is loopback (or `DCALCON_METRICS_TOKEN`); sample Caddyfiles do not proxy it.

## SQLite

Fine for one host. Not for a herd of writers. `dcalcon serve` is the production shape (WAL, busy timeout, one writer). Split compose still shares one file on one node.

If you ever need multiple app nodes, swap the store (Postgres), not the DAV paths.

## Mail and “connected accounts”

Gmail still has CalDAV-via-OAuth; this project uses Gmail send for iMIP instead. Microsoft 365 has no CalDAV — Graph Mail.Send only. Connecting an account means OAuth apps you register, tokens sealed in SQLite, invites mailed as `text/calendar`. Users of *this* server still sync with CalDAV on their devices.

## Config

Mostly env (optional YAML: `src/svc/configs/dcalcon.example.yaml`). Copy `.env.example` for compose.

| Variable | |
|---|---|
| `DCALCON_CONFIG` | YAML path |
| `DCALCON_HTTP_ADDR` | listen (`:8080`) |
| `DCALCON_PUBLIC_URL` | public origin |
| `DCALCON_SQLITE_PATH` | database file |
| `DCALCON_ADMIN_USERNAME` / `DCALCON_ADMIN_PASSWORD` | first boot; password cannot be `changeme` |
| `DCALCON_SESSION_SECURE` | `true` behind TLS (`https://` public URL also forces this) |
| `DCALCON_CORS_ORIGINS` | extra dashboard origins |
| `DCALCON_BACKUP_DIR` / `DCALCON_BACKUP_HOOK` | snapshots; hook gets the file path |
| `DCALCON_AUTH_MAX_ATTEMPTS` / `WINDOW` / `LOCKOUT` | login and DAV Basic |
| `DCALCON_TOKEN_KEY` | AES-GCM for OAuth/SMTP/TOTP (hex or passphrase; file next to the DB if empty) |
| `DCALCON_SMTP_*` | recovery mail and iMIP fallback; without it, copy the link from Users |
| `GOOGLE_OAUTH_*` / `MICROSOFT_OAUTH_*` | iMIP. Redirect `{origin}/api/v1/oauth/{provider}/callback` |

## License

Not chosen yet.
