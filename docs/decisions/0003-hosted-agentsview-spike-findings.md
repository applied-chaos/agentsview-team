# Decision: Run hosted Instances under a custom seccomp profile; tee uploads for the Admin Instance

Status: accepted
Date: 2026-11-14

## Context
ADR 0001 and ADR 0002 left four questions to a spike (ticket #2): does upstream's sandboxed hosted parser run inside the Platform container, does provisioning work with a database per Instance and no superuser, what does an Instance cost, and does replaying committed raw objects give the Admin Instance the same sessions.

Evidence comes from `spike/` (see its README): a stand-in Platform container running agentsview v0.47.0 Instances as child processes against `postgres:17`, with synthetic Claude Code sessions uploaded by the real `agentsview source raw backfill` client. Runs: Docker Desktop 29.8.1 on arm64 (LinuxKit 7.0, no AppArmor) and rapunzel (Docker 27.2.0, Ubuntu 22.04, kernel 5.15, amd64, AppArmor), as a separate compose project; the production stack was not touched and the spike was removed afterwards.

### 1. Sandbox
- Docker's default seccomp profile fails closed on both hosts: `hosted parser isolation unavailable`. There is no fallback parser.
- Works with Docker's default profile plus `clone`, `mount` and `chroot` allowed unconditionally (`spike/mkseccomp.py`). `clone` alone or `mount` alone still fails. `chroot` is only needed because capabilities are dropped (the default profile gates it on `CAP_SYS_CHROOT`).
- AppArmor `docker-default` blocks it even with that profile (it denies `mount`); `apparmor=unconfined` works on rapunzel. A custom AppArmor profile allowing `mount` is unproven.
- No capabilities needed. Proven with uid 10001, `cap_drop: ALL`, `no-new-privileges`, read-only root. Needs unprivileged user namespaces (rapunzel: `unprivileged_userns_clone=1`, `max_user_namespaces=63485`).
- Read-only root needs a writable passwd home (Docbank keeps vault locks in `~/.local/state/docbank`, ignoring `$HOME`; otherwise raw-sync returns 500) and a writable `TMPDIR` for parser materialisation. A tmpfs `/tmp` broke raw-sync uploads; cause not isolated, so `TMPDIR` points at the data volume.
- arm64 executes, although upstream only claims compile proof there.
- EKS (unproven, not run): pod `seccompProfile: Localhost` with this profile installed on every node, `runAsNonRoot`, drop `ALL`, `allowPrivilegeEscalation: false`. That fits the restricted Pod Security Standard, which forbids `Unconfined`. Nodes must allow user namespaces (check `user.max_user_namespaces` on the chosen AMI). AppArmor nodes (Ubuntu) need a Localhost AppArmor profile; Bottlerocket's SELinux policy is untested. Fargate cannot load Localhost seccomp profiles, so it is out. containerd's `RuntimeDefault` derives from the same rules as Docker's and will fail like it.

### 2. Provisioning without superuser
- Proven on PostgreSQL 17 with one superuser step: creating `platform` as `LOGIN CREATEDB CREATEROLE NOSUPERUSER NOBYPASSRLS NOREPLICATION` (Aurora master analogue). Per Instance, as `platform`: owner role, runtime role (`NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT`), `CREATE DATABASE … OWNER <owner>`. On PostgreSQL 16+ that needs `GRANT <owner> TO platform WITH INHERIT FALSE, SET TRUE` first. As owner: `REVOKE ALL ON DATABASE … FROM PUBLIC`, `GRANT CONNECT` to the runtime role, `CREATE SCHEMA`, `hosted-provision`, runtime grants.
- `pg_trgm`: `hosted-provision` installs it into `public` as the database owner. It is a trusted extension and the owner holds `CREATE` on `public` through `pg_database_owner`. No superuser and no manual step. Aurora itself is unproven; it lists `pg_trgm` as supported.
- Upstream's documented runtime grants are incomplete (`excluded_sessions` needs `UPDATE`, among others); start-up refuses with `owner provisioning required`. The authoritative list is `hostedProjectionPrivileges` in `internal/postgres/raw_runtime_privileges.go`; the Platform must take its grants from there and re-check on every upgrade.
- Enrolment is an `INSERT` into `raw_devices` (SHA-256 of an `avdc_` credential) as the owner, after `SET agentsview.tenant_id` because every table has forced row-level security. The runtime role cannot enrol, by design.
- Re-provisioning existing Instances on every start is a no-op, as ADR 0001 needs.
- An empty Instance database is 11 MB.

### 3. Cost per Instance
Memory is PSS (the binary's shared pages split between processes); anon is private heap.

| State | CPU (one core = 100%) | Memory |
| --- | --- | --- |
| Idle, fresh (20 Instances, arm64) | 0.1% each | 44 MB PSS, 40 MB anon each; container 830 MiB total |
| Idle, fresh (rapunzel) | 0.0% | 52 MB PSS, 29 MB anon |
| Idle after a backfill, until restart | 1.5 to 2.2% | 83 to 112 MB PSS |
| Backfill, 500 sessions of 75 KB | mean 6%, peak 48% | peak 153 MB PSS |
| Backfill, 9.5 MB sessions (6,000 messages) | peak 108% (arm64), 130% (rapunzel) | peak 213 to 243 MB PSS |

- Parsing is one job per poll tick per Instance (`BatchSize: 1` is hard-coded; `raw_poll_seconds` defaults to 5, minimum 1). 500 files took 16.7 minutes at 2 seconds. Backfill speed is set by the poll interval, not CPU.
- Each Instance holds one idle Postgres connection, and its pool allows up to 5.

Budget for 100 Instances plus the Admin Instance:
- Platform container: about 4.5 GiB for idle Instances, plus up to about 250 MB for each Instance parsing at once. The parser child may reach its 1 GiB data limit, so a 10-way burst needs another 2.5 to 10 GiB. Request 6 GiB, limit 12 GiB.
- CPU: 0.1 to 2 cores idle (higher after activity), plus one core per Instance parsing at once. Request 2, limit 4.
- Postgres: `max_connections` of at least 600 (5 × 101 plus the Platform), about 1.1 GB of empty catalogues, plus session data.
- rapunzel has about 5 GB free memory today, so it can host tens of Instances, not 100.

### 4. Replay to a second Instance
- Proven: every committed manifest of one Instance (two generations per source, 50 sources), re-uploaded with upstream's own raw-sync client to a second Instance as one mirrored device, gives identical sessions on both hosts. The comparison covered source session ID, project, times, message and tool-call counts, tokens, health, outcome and a hash of every message. API session IDs are the same too.
- Differences: `source_machines` names the mirrored device; parse-job counts differ (the source Instance parsed some first generations before the second arrived, while replay arrives in a burst), with no effect on sessions.
- The source Instance's raw object vault is exclusively locked while it runs (`vault is locked by another process`). ADR 0002's worker reading objects from the Instance's data directory therefore cannot run beside the Instance; the spike stops the source Instance to replay.
- Manifests are readable from Postgres (`raw_manifests.canonical_json`). Replay must replace `expected_parent_receipt` with the Admin Instance's own receipt chain per source, and keep that chain durably to resume.
- The replay tool imports upstream `internal/` packages, so it builds only inside upstream's module. A Platform in its own module must implement the raw-sync client protocol itself (token exchange, missing objects, resumable upload, manifest commit) or vendor it.

## Decision
- The Platform container runs with the custom seccomp profile from `spike/mkseccomp.py`, AppArmor unconfined where AppArmor exists, non-root, all capabilities dropped, `no-new-privileges`, read-only root with a writable home and `TMPDIR`. Docker deployments pass these in compose; EKS uses a Localhost seccomp profile.
- Provisioning follows section 2 with an Aurora-like `platform` role. Runtime grants come from upstream's code list, pinned with the agentsview version.
- Instances use `raw_poll_seconds = 1` so backfill reaches 3,600 files an hour.
- The Admin Instance is fed by the Platform's raw-sync pass-through: it keeps a verified copy of each uploaded object (content-addressed by SHA-256 and length) and each manifest in its own spool on the data volume. After the Instance answers a manifest commit with a receipt, it replays them to the Admin Instance as the mirrored device. This replaces reading the Instance's data directory. The tee is unproven in code, but it sends the same requests as the proven replay.

## Consequences
- The sandbox relies on a seccomp profile the Platform ships and a host or node that allows user namespaces and either has no AppArmor or runs it unconfined. Platforms that only offer default runtime profiles (Fargate, locked-down clusters) cannot run Instances.
- AppArmor unconfined weakens rapunzel's container confinement; a custom AppArmor profile is follow-up work.
- Upgrades must diff upstream's grant list and seccomp needs; both are upstream internals, like the device table.
- The Platform needs owner access per Instance for provisioning and enrolment, besides the runtime credentials the Instance uses.
- The Platform's tee spool duplicates raw storage a third time until replayed. It can delete an object once the Admin Instance has committed every manifest that references it.
- Memory, not CPU, bounds Instances per container; 100 Instances need a host or node with at least 8 to 16 GiB for the Platform.
- Idle CPU after activity (1.5 to 2% per Instance, gone after restart) has no cause yet; watch it before sizing CPU down.

## Supersedes
ADR 0002 in part: the replay worker's source changes from "the raw objects in that Instance's data directory" to the Platform's own copy taken at the raw-sync pass-through. Resolves the open consequences in ADR 0001 (namespaces and seccomp in the Platform container) and ADR 0002 (replayed objects give the same sessions).
