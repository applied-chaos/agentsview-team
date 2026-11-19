# Platform mockups

UI acceptance reference for the AgentsView Teams Platform (ticket #3, spec #1). Static HTML sharing one token stylesheet (`mock.css`); `mock.js` draws the top bar, slim Platform bar, banners and Settings dialog, and renders the activity calendar and chart. Load `mock.js` in `<head>` so theme and accent apply before first paint. Data is fictional.

Start at `index.html` (gallery of every screen and state). `?theme=light|dark` forces a theme; the account menu switches theme and accent live. Dialog states open by URL hash. `./shoot.sh` regenerates `png/` (desktop 1440x900 and mobile 390x844, light and dark); pass page states to render a subset.

| Spec screen | File | States |
|---|---|---|
| Sign in / not permitted / errors | `sign-in.html`, `not-permitted.html`, `error.html` | `?reason=` any error page the Platform serves |
| Overview (landing) | `overview.html`, `overview-empty.html`, `overview-loading.html` | `#settings`, `?menu=open` (mobile), first run, loading placeholders |
| Hosts (incl. Install Tokens) | `hosts.html`, `hosts-loading.html` | `#dlg-token`, `#dlg-created` (shown once), loading placeholders |
| Add Host | `add-host.html` | OS switch, waiting for Host, `#connected` |
| Host detail | `host.html` | Offline, outdated, `#dlg-revoke` |
| Team | `team.html` | |
| My Instance with Platform bar | `instance.html`, `instance-failed.html` | agentsview body is a stand-in |
| Instance View and Impersonation banners | any page with `?banner=view` or `?banner=imp` | |
| Admin Users list | `admin-users.html` | Disabled, pending Admin grant, Instance failed, `#dlg-grant` |
| Admin User detail | `admin-user.html` | `#dlg-disable`, `#dlg-delete` |
| Admin Instance and per-User breakdown | `admin-instance.html` | |
| Audit Log | `audit.html` | |

Rules the build must keep: status is dot plus word, never colour alone; Admin lives in the account menu, not a tab; banners stay pinned while scrolling; secrets show once with a warning; the Team page never shows another person's figures; accent defaults to `#3b5bdb` and is per User.

Mockup-only shortcuts the build must not copy: dialogs open via `:target`, so they do not move or trap focus; fonts load from Google Fonts by `@import`.
