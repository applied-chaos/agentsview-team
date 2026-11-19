# agentsview-team

A team front end for [agentsview](https://github.com/kenn-io/agentsview): a sign-in site where each User gets their own private agentsview Instance, their machines (Hosts) push coding-agent sessions to it, and Admins get a team-wide view through an Admin Instance.

```
Host ──HTTPS──▶ your reverse proxy (TLS) ──▶ platform (Instances as child processes, /data)
browser ──────▶                                   │
                                                  ▼
                                             postgres:17
```

Images: `ghcr.io/applied-chaos/agentsview-team`, for `linux/amd64` and `linux/arm64`. Tags: `vX.Y.Z` per release, `latest` (newest release), `main`, `sha-<7 chars>`. Release tags and `main` are rebuilt weekly to pick up base image fixes, so pin a release tag, not a digest, to get them.

## Deploy with Docker Compose

### Requirements

- Docker with Compose v2, on Linux.
- Unprivileged user namespaces enabled on the host (`sysctl kernel.unprivileged_userns_clone` is `1` where it exists; `user.max_user_namespaces` above 0; on Ubuntu 23.10 and later also `kernel.apparmor_restrict_unprivileged_userns=0`). Instances parse sessions in a sandbox that needs them ([ADR 0003](docs/decisions/0003-hosted-agentsview-spike-findings.md)).
- An OIDC issuer (Authentik, Keycloak, Google, Entra ID, ...) and a client registered with redirect URI `https://<your domain>/auth/callback`.
- A reverse proxy terminating TLS in front of port 8080. Hosts refuse plain HTTP.
- Memory: roughly 50 MB per idle Instance and 250 MB per Instance during a backfill.

### Steps

1. Copy the [`deploy/`](deploy) directory to the server (say `/opt/agentsview-team`): `compose.yaml`, `seccomp.json`, `initdb/platform.sh`, `.env.example`.
2. `cp .env.example .env`, `chmod 600 .env`, and fill it in. Generate each secret with `openssl rand -hex 32`. Pin `PLATFORM_VERSION` to a release tag.
3. Create the data directories: `mkdir pgdata instances && sudo chown 10001 instances` (the image runs as uid 10001).
4. `docker compose up -d --wait`
5. Point your reverse proxy at `127.0.0.1:8080`. Disable response buffering so Instance streams flow, for example with Caddy:

   ```
   agentsview.example.com {
   	reverse_proxy 127.0.0.1:8080 {
   		flush_interval -1
   	}
   }
   ```

6. Sign in once, then make yourself an Admin: `docker compose exec platform platform admin grant <email>`.

On first start Postgres runs `initdb/platform.sh`, which creates the Platform's own role (no superuser) and database. It only runs on an empty `pgdata`.

### What the compose file sets, and why

The Platform runs Instances under a sandbox that needs exactly this, and no more:

- `seccomp=seccomp.json`: Docker's default profile plus `clone`, `mount` and `chroot`. The default profile makes Instances fail with `hosted parser isolation unavailable`.
- `apparmor=unconfined`: Docker's default AppArmor profile denies `mount`.
- Non-root, `cap_drop: ALL`, `no-new-privileges`, read-only root with a tmpfs home.

### Configuration

All set in `.env`:

| Variable | Required | Meaning |
| --- | --- | --- |
| `PLATFORM_VERSION` | yes | Image tag to run. |
| `PLATFORM_BASE_URL` | yes | Public HTTPS URL. |
| `POSTGRES_PASSWORD` | yes | Postgres superuser password. |
| `PLATFORM_DB_PASSWORD` | yes | Password of the Platform's `platform` role. |
| `PLATFORM_SECRET_KEY` | yes | Seals Instance secrets and signs sessions. Back it up; losing it makes sealed secrets unreadable. |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | yes | OIDC client. |
| `OIDC_PROVIDER_NAME` | no | Shown on the sign-in button. Default `SSO`. |
| `OIDC_SCOPES` | no | Default `openid email profile`. |
| `OIDC_GROUPS_CLAIM` | no | Claim holding groups. Default `groups`. |
| `PLATFORM_ALLOWED_EMAIL_DOMAINS` | no | Comma-separated domains allowed to sign in. |
| `PLATFORM_REQUIRED_GROUP` | no | Group a User must be in to sign in. |
| `PG_MAX_CONNECTIONS` | no | Postgres `max_connections`, five per Instance plus the Platform. Default `400`. |

With neither `PLATFORM_ALLOWED_EMAIL_DOMAINS` nor `PLATFORM_REQUIRED_GROUP` set, every login the issuer authenticates is accepted.

## Operating

### Upgrade

Change `PLATFORM_VERSION` in `.env`, then `docker compose up -d --wait`. Each Host page shows an install command under "Reinstall or update"; re-running it on a Host upgrades it in place, keeping its credential, name and install mode.

### Admins

`docker compose exec platform platform admin grant <email>` and `... admin revoke <email>`. The CLI can revoke the last Admin (the UI refuses), so a mistake can be undone.

### Rotate the secret key

```sh
docker compose stop platform
docker compose run --rm -e PLATFORM_OLD_SECRET_KEY=<old> -e PLATFORM_SECRET_KEY=<new> platform secrets rotate
# put <new> in .env as PLATFORM_SECRET_KEY
docker compose up -d --wait
```

`secrets rotate` re-seals every sealed secret and is safe to repeat. Signed-in Users are signed out.

### Backup and restore

State is every Postgres database (`platform`, one per Instance, the Admin Instance) plus `instances/`. Take both while the Platform is stopped, so the Admin Instance spool and the replay tables agree (Hosts retry uploads meanwhile). Keep `.env` too, for `PLATFORM_SECRET_KEY`.

Backup:

```sh
mkdir -p backup && d=$(date +%F)
docker compose stop platform
docker compose exec -T postgres pg_dumpall -U postgres | gzip > backup/pg-$d.sql.gz
sudo tar czf backup/instances-$d.tar.gz -C instances .
docker compose start platform
```

Restore (replaces current state; use the same `PLATFORM_SECRET_KEY`):

```sh
docker compose down
sudo find pgdata instances -mindepth 1 -delete
docker compose up -d --wait postgres
gunzip -c backup/pg-<date>.sql.gz | docker compose exec -T postgres psql -U postgres -d postgres -q   # "role postgres already exists" is expected
sudo tar xzf backup/instances-<date>.tar.gz -C instances
docker compose up -d --wait
```

### Host logs

A Host's watcher logs each initial backfill and its own start. Linux with systemd: `journalctl --user -u agentsview-host`. Linux shell-profile mode and macOS: `~/.local/share/agentsview-platform/agent.log`. Windows: `%LOCALAPPDATA%\agentsview-platform\agent.log`. Raw backfill output sits beside it in `backfill.log`. A large history takes a while: about 10 minutes for 2.6 GB of Claude sessions.

## Development

Go service plus Svelte SPA in one image. Needs Go, Node 22, Docker and `shellcheck`.

- `make check`: the full gate (formatting, linters, unit tests, compose e2e, Playwright).
- `make image`: builds `agentsview-team:local`.
- Design notes: [PRODUCT.md](PRODUCT.md), [DESIGN.md](DESIGN.md), [GLOSSARY.md](GLOSSARY.md), [decisions](docs/decisions).

## Licence

[MIT](LICENSE).
