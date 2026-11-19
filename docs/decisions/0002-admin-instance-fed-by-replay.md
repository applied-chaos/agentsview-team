# Decision: Admin Instance fed by replaying uploads from disk

Status: accepted, superseded in part by ADR 0003
Date: 2026-11-14

## Context
Admins need one agentsview across every User's sessions, and Users need organisation-wide totals. Upstream has no replication between instances and no cross-tenant reads. Raw-sync requests are signed and content addressed, and the server stores raw objects on its local filesystem.

## Decision
Hosts upload once, straight through the Platform to their User's Instance. After each committed manifest, a Platform worker re-uploads the new raw objects from that Instance's data directory to the Admin Instance, as a mirrored device named `<user email>/<host name>`. Deletes in a User's Instance are not propagated; the Admin Instance keeps everything.

Superseded by ADR 0003: the worker replays the Platform's own copy of each upload, taken at the raw-sync pass-through, not the Instance's data directory, which a running Instance locks.

Rejected: Hosts uploading twice (double bandwidth and Credentials, Admin copy depends on the Host); the Platform implementing the raw-sync server itself (duplicates upstream's protocol); no Admin Instance, with the Platform querying every Instance (no agentsview UI across Users).

## Consequences
- Raw storage roughly doubles.
- The Platform implements a raw-sync client; content addressing makes replay idempotent across retries and restarts.
- Replayed objects produce the same sessions as in the User's Instance (proven in ADR 0003).
- A session a User deletes stays visible to Admins and on the Team page.

## Supersedes
None
