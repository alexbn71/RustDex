<p align="center"><img src="web/icon.svg" width="96" alt="RustDex icon"></p>

# RustDex

A tiny, self-hosted **address book server for [RustDesk](https://rustdesk.com)**.

RustDex speaks the part of the RustDesk *API server* protocol that the client uses for its address book, and nothing else. Point the RustDesk client at it (port **21114**, like [rustdesk-api](https://github.com/lejianwen/rustdesk-api)) and your list of computers, with their IDs and passwords, is shared by every RustDesk you log in from. A small web page on the same port manages the list, with contact details for each computer (company, contact person, email, phone, address, notes), in the spirit of the Supremo address book.

- One static Go binary with SQLite storage, in a `FROM scratch` Docker image (amd64 and arm64)
- One admin account, configured with environment variables
- Passwords encrypted at rest (AES-256-GCM)
- Web UI in English and Italian, light and dark theme, works on phones
- **Connect** button that opens RustDesk with the ID and the saved password (`rustdesk://` link)

**What it does not do.** RustDex is not a replacement for the RustDesk ID/relay server (`hbbs`/`hbbr`): keep running those. It also leaves out everything else rustdesk-api offers: multiple users, groups, device management, audit logs, OAuth/OIDC, web client.

## Quick start

```yaml
# docker-compose.yml
services:
  rustdex:
    image: ghcr.io/alexbn71/rustdex:latest
    container_name: rustdex
    restart: unless-stopped
    ports:
      - "21114:21114"
    environment:
      RUSTDEX_ADMIN_USER: admin
      RUSTDEX_ADMIN_PASSWORD: change-me
    volumes:
      - rustdex-data:/data

volumes:
  rustdex-data:
```

```sh
docker compose up -d
```

Open `http://<server>:21114` and log in. If `RUSTDEX_ADMIN_PASSWORD` is empty, a password is generated on first start and printed **once** in the log (`docker logs rustdex`).

## Configure the RustDesk client

1. In RustDesk open **Settings → Network → ID/Relay server** and set **API server** to `http://<server>:21114` (keep your ID server and key as they are).
2. Log in with the RustDex account (the account button on the main screen, or Settings → Account).
3. Open the **Address book** tab and pick **RustDex** from the address book list (next to *My address book*).

The RustDex book carries the passwords, so connecting from the address book needs no typing. Changes made in the client (alias, tags, notes, password) are saved to RustDex and show up in the web UI, and the other way round. Contact details (company, contact person, email, phone, address) are visible only in the web UI, because the RustDesk client has no place for them.

*My address book* (the personal book) also works and is kept by RustDex, but it only stores password hashes, as the RustDesk client wants; it is not shown in the web UI.

**Computers added automatically.** When you connect to a computer that is not in the address book yet, RustDex adds it with the tag `New` (`RUSTDEX_AUTO_ADD_TAG`): filter by that tag in the web UI, fill in the name and contact details, and remove the tag. This relies on the *remote* computer reporting the connection (the audit record RustDesk sends to its API server), so it works for computers whose RustDesk uses RustDex as API server. That includes those that only set your ID server: RustDesk then uses port 21114 on the same host as API server, so run RustDex on the ID server host at port 21114. Only connections made from a RustDesk client logged in to RustDex add computers; connections by anyone else are ignored.

**Online status.** Computers whose own RustDesk is configured with this API server send a heartbeat: the web UI shows them as *Online* and fills in the computer name, user and operating system. The others show `—`.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `RUSTDEX_ADMIN_USER` | `admin` | Login name |
| `RUSTDEX_ADMIN_PASSWORD` | *(generated)* | Login password. Empty: generated on first start, printed once in the log, stored hashed |
| `RUSTDEX_LISTEN` | `:21114` | Listen address |
| `RUSTDEX_DATA_DIR` | `/data` | Database and key folder |
| `RUSTDEX_SECRET_KEY` | *(key file)* | Passphrase for the password encryption. Empty: a random key is created in `/data/secret.key` |
| `RUSTDEX_BOOK_NAME` | `RustDex` | Name of the address book shown in the RustDesk client |
| `RUSTDEX_SESSION_DAYS` | `30` | Login lifetime, extended while in use |
| `RUSTDEX_TRUST_PROXY` | `false` | Behind a reverse proxy: take the client address from `X-Real-IP` / `X-Forwarded-For` (used by the login lockout) |
| `RUSTDEX_AUTO_ADD` | `true` | Add the computers you connect to (see below) |
| `RUSTDEX_AUTO_ADD_TAG` | `New` | Tag given to the computers added automatically; `-` for none |
| `PUID` / `PGID` | `65532` | User and group that own the data folder and run the server (Unraid: `99` / `100`) |

### Data and backups

Everything is in the data folder: `rustdex.db` (SQLite) and `secret.key`. Back up **both**: the database alone does not reveal the passwords, but it cannot be read back without the key either. If you set `RUSTDEX_SECRET_KEY`, keep it somewhere safe instead of the key file.

The container starts as root only to give the data folder to `PUID`:`PGID` (default `65532:65532`), then drops to that user, so bind-mounted folders created by root work too. On **Unraid** set `PUID=99` and `PGID=100` (nobody:users, like the other appdata folders). `PUID=0` keeps it running as root; starting it with `--user` skips the switch, and then that user must be able to write the folder.

## Security notes

- Like rustdesk-api, RustDex listens on plain HTTP, so logins and passwords travel unencrypted. On a LAN or VPN that may be fine. If it is reachable from the internet, put it behind a reverse proxy with HTTPS (Nginx Proxy Manager, Caddy, Traefik…) and set `RUSTDEX_TRUST_PROXY=true`.
- After 5 failed logins from an address, logins from that address are refused for 15 minutes.
- Sessions are stored as SHA-256 hashes. Logging out from the client or the web UI ends that session.

## How it works

RustDex implements these endpoints of the RustDesk API server:

| Endpoint | Purpose |
|---|---|
| `POST /api/login`, `/api/logout`, `/api/currentUser`, `GET /api/login-options` | Account |
| `POST /api/ab/settings`, `/api/ab/personal`, `/api/ab/shared/profiles` | Address book discovery |
| `POST /api/ab/peers`, `/api/ab/tags/{guid}` | Read peers and tags |
| `POST /api/ab/peer/add/{guid}`, `PUT /api/ab/peer/update/{guid}`, `DELETE /api/ab/peer/{guid}` | Edit peers |
| `POST /api/ab/tag/add/{guid}`, `PUT /api/ab/tag/rename/{guid}`, `PUT /api/ab/tag/update/{guid}`, `DELETE /api/ab/tag/{guid}` | Edit tags |
| `POST /api/heartbeat`, `/api/sysinfo`, `/api/sysinfo_ver` | Online status and computer info |

The legacy address book API (`GET/POST /api/ab`, used by very old clients) is not implemented. The web UI uses its own API under `/rx/api/`.

## Development

Requires Go 1.26.

```sh
go test ./...
RUSTDEX_DATA_DIR=./data RUSTDEX_ADMIN_PASSWORD=dev go run .
docker build -t rustdex .
```

The web UI (`web/`) is plain HTML, CSS and JavaScript with no build step, embedded in the binary.

## License

[MIT](LICENSE). RustDex is an independent project, not affiliated with RustDesk.
