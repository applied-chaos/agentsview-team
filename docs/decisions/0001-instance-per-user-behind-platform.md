# Decision: One agentsview Instance per User, behind the Platform

Status: accepted
Date: 2026-11-14

## Context
agentsview has no notion of users: its UI uses one shared bearer token, its hosted raw sync serves exactly one tenant per server process (own schema, restricted role, forced RLS), and sessions carry only a machine or device. We need 10 to 100 Users in one organisation to each see only their own sessions, signed in through a configurable OIDC provider.

## Decision
Each User gets their own Instance: one agentsview server process, run always-on as a supervised child process of the Platform, with its own Postgres database and login role on one shared Postgres server (plus a `platform` database for the Platform). The Platform is the only public entry point: it signs Users in (OIDC, short-lived JWT access tokens and refresh tokens that re-check the User), and reverse-proxies each Instance under `/i/<user-id>/`, adding that Instance's bearer token. Hosts are raw-sync devices in their User's Instance, enrolled and revoked by the Platform writing upstream's device records, since upstream has no enrolment endpoint.

Rejected: one shared instance with the Platform filtering API calls (device filters are query options, not access control); schema per Instance in one database (weaker isolation, harder per-User delete); container per Instance (needs the Docker socket, root-equivalent); patching upstream for multi-tenancy.

## Consequences
- Isolation enforced by Postgres connect privileges and upstream's own tenant protections.
- Memory and processes grow with Users; one Platform replica only, since it owns every Instance. Sharding Users across pods is a future concern.
- The Platform depends on upstream internals (device table, provisioning, schema version): agentsview is pinned in the image and upgraded deliberately, with per-Instance provisioning upgrades at start-up.
- Server-side parsing needs Linux namespaces and seccomp inside the Platform container; to be proven in the spike.
- Users fully control their Instance, including deleting sessions.

## Supersedes
None
