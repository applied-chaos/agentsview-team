# agentsview Platform

A sign-in site in front of agentsview that gives each person in one organisation their own agentsview of their machines' coding-agent sessions, and gives Admins a view across everyone.

## Language

**Platform**:
The site people sign in to; it manages Users and Hosts and is the only way into any Instance.
_Avoid_: Portal, orchestrator, control plane

**User**:
A person in the organisation who signs in, owns Hosts and has one Instance.
_Avoid_: Account, member, tenant

**Admin**:
A User who may also manage every User, view any Instance, Impersonate any User and see the Admin Instance.
_Avoid_: Operator, superuser

**Disabled User**:
A User who can no longer sign in and whose Hosts are revoked, but whose sessions are kept.
_Avoid_: Deleted user, suspended

**Host**:
A machine, as one login on it, owned by one User, that sends that login's agent sessions to the Platform.
_Avoid_: Machine, device, node, client

**Ephemeral Host**:
A Host for a short-lived machine, such as a cloud workspace or CI runner, made from an Install Token marked Ephemeral and revoked automatically once unseen for long enough.
_Avoid_: Temporary host, cloud host

**Credential**:
The secret one Host presents to send sessions; each Host has exactly one.
_Avoid_: Password, API key

**Install Token**:
A secret, valid for 1 hour, 1 day, 7 days, 30 days or never (the User chooses), that lets machines become Hosts of the User who made it; either single-use or reusable, and optionally making its Hosts Ephemeral Hosts.
_Avoid_: Enrolment key, invite

**Instance**:
A User's own agentsview, holding only that User's sessions, which the User fully controls, including deleting sessions.
_Avoid_: Tenant, server, workspace

**Admin Instance**:
The agentsview holding every User's sessions, each marked with its User, seen only by Admins; deleting a session in an Instance leaves it here.
_Avoid_: Global instance, aggregate

**Team**:
The page of organisation-wide totals from the Admin Instance that every User may see, never broken down by person.
_Avoid_: Org Summary, leaderboard, global stats

**Instance View**:
An Admin opening a User's Instance as themselves, with the same control the User has.
_Avoid_: Spectate, peek

**Impersonation**:
An Admin acting as a User across the Platform, with every action recorded as the Admin on that User's behalf.
_Avoid_: Sudo, switch user

**Audit Log**:
The Admin-only record of every Impersonation and Admin action.
_Avoid_: History, activity log
