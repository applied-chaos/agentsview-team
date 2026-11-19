# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Svelte SPA built into the Platform image (spec #1). Mockups: static HTML per screen sharing one token stylesheet, plus committed desktop (1440) and mobile (390) PNGs, in `docs/mockups/`.

## Users

- **Users**: 10 to 100 people in one organisation, mostly engineers using coding agents across laptops, servers, cloud workspaces and CI. They sign in with the organisation's OIDC provider, add their own Hosts and read their own sessions.
- **Admins**: a few Users with the Admin role who manage Users, read across the organisation through the Admin Instance, Impersonate and audit.
- **Operator**: deploys and runs the container (CLI, environment config); no UI.

## Product Purpose

AgentsView Teams is a sign-in site in front of agentsview. Each User gets a private Instance (their own agentsview with only their sessions). Users enrol Hosts by copying one install command. Uploads are mirrored into an Admin Instance for organisation-wide visibility and the Team page. Success: a new User signs in and has a Host uploading within minutes; Admins can see and support everyone without a shared token.

## Positioning

Per-User isolation enforced by Postgres (one Instance and database per User) rather than UI filtering, with self-service Host enrolment modelled on Tailscale auth keys.

## Operating Context

Daily, brief visits: check Hosts are Online, add a machine, copy an install command into a terminal, open My Instance. Admins visit occasionally for support, user lifecycle and audit. Instance pages are agentsview itself, full page, under a slim Platform bar.

## Capabilities and Constraints

- Vocabulary follows `GLOSSARY.md` (User, Admin, Instance, Admin Instance, Host, Install Token, Ephemeral, Instance View, Impersonation, Audit Log, Team).
- Screens (spec #1): Sign in / not permitted; Overview (landing); Hosts (incl. Install Tokens); Add Host (incl. token dialog); Host detail; Team; My Instance with Platform bar; Admin Users list; Admin User detail; Admin Instance entry and per-User breakdown; Instance View and Impersonation banners; Audit Log.
- The Team page never breaks figures down by person.
- Secrets (Host Credentials, Install Tokens) shown once only.
- Host status: Online (seen within 30 min), Offline, Revoked; outdated client flagged.

## Brand Commitments

- Name: **AgentsView Teams**.
- Standard light/dark SaaS admin, not agentsview's look: Tailscale admin console as the main reference, Linear/Vercel as the craft bar. Full dark theme with a toggle in the account menu.
- One slim top bar (brand, Overview / Hosts / Team, My Instance; account menu with Settings, Admin, Dark mode, Sign out) so it sits comfortably alongside agentsview.
- Source Sans 3 (JetBrains Mono for commands and tokens), 15px body, few sizes, striped lists rather than boxes, no nested borders, generous spacing.
- Accent defaults to blue `#3b5bdb`; each User can pick another from a palette or set HSL in Settings.

## Evidence on Hand

- Reference screenshots of agentsview v0.47.0 (sessions, usage, mobile) taken from a local instance.
- No real organisation names, users or usage numbers: mockups use clearly fictional sample data.

## Product Principles

- Self-service first: a User never needs an Admin or Ansible to add a machine.
- Privacy by structure: never imply one User can see another's data unless an Admin is explicitly acting as them.
- Admin power is always visible: Impersonation and Instance View are never ambiguous.
- Calm and readable over dense: one task per screen, the few things a visit needs, nothing more.

## Accessibility & Inclusion

WCAG 2.2 AA; usable on phone width; keyboard-operable; status never conveyed by colour alone.
