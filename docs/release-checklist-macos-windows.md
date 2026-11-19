# Release checklist: macOS and Windows Hosts

Automated gates (`make check`) only lint these scripts (shellcheck, PSScriptAnalyzer) and test that the Platform serves them. Run this by hand on real machines before each release and whenever the pinned agentsview version or an install script changes.

Needs: a macOS machine (Apple Silicon; Intel if available), a Windows 11 machine (PowerShell 5.1 and 7), a deployed Platform build, a signed-in test User.

## macOS

1. Add Host, pick macOS, copy the command. Run it in Terminal.
2. Output ends "Installed as the launchd user agent". `launchctl print gui/$(id -u)/net.mintopia.agentsview-host` shows state running.
3. `~/.local/bin/agentsview --version` prints the pinned version. Files exist: `~/.config/agentsview-platform/host.env` (mode 600), `~/.local/share/agentsview-platform/run.sh`, `~/Library/LaunchAgents/net.mintopia.agentsview-host.plist`.
4. Add Host page shows "connected" and Online within a minute. Existing Claude/Codex sessions reach the Instance (backfill).
5. Idempotent: run the same command again (token already spent). No prompt, same Credential in `host.env`, one Host in the list, agent still running.
6. Survives restart: `launchctl kill TERM gui/$(id -u)/net.mintopia.agentsview-host`; the agent comes back within ~15 s. Log out and in, or reboot: agent running, Host Online.
7. Upgrade: point at a Platform with a newer pinned version, re-run; binary replaced, agent restarted, Host row shows the new version after re-enrol only (known: no check-in yet).
8. Bad token (`AVT_TOKEN=avt_tk_nope`) on a clean user: fails with the Platform's message, nothing installed under `~/.config/agentsview-platform`.
9. Uninstall: `curl -fsSL <platform>/install/macos-uninstall.sh | bash`. Agent gone from `launchctl list`, the plist, `~/.local/bin/agentsview`, config and share dirs removed. Running it twice is harmless.
10. Over SSH with no GUI session: install prints the "starts at your next login" message and exits 0.

## Windows

Repeat on Windows PowerShell 5.1 and PowerShell 7, as a standard (non-admin) user.

1. Add Host, pick Windows, copy the command. Run it in PowerShell. The shell stays open afterwards (no `exit`).
2. Output ends "Installed as the Scheduled Task agentsview-host". `Get-ScheduledTask agentsview-host` is Running, trigger At log on, settings: restart every 1 minute up to 999 times, no time limit, runs on battery.
3. `& "$env:LOCALAPPDATA\agentsview-platform\bin\agentsview.exe" --version` prints the pinned version. `host.env`, `run.ps1`, `data\` exist beside it.
4. Add Host page shows "connected" and Online within a minute; existing sessions arrive.
5. Idempotent: run the command again. No prompt, same Credential, one Host, task still registered once and running.
6. Failure restart: end the `agentsview` process in Task Manager; the watcher process reappears within about two minutes and the Host stays Online.
7. Log off and on (and reboot): task starts at logon, Host Online.
8. Upgrade as macOS step 7.
9. Bad token: throws with the Platform's message, no task registered.
10. Uninstall: `irm <platform>/install/windows-uninstall.ps1 | iex`. Task gone, `%LOCALAPPDATA%\agentsview-platform` removed, no `agentsview` process left. Second run harmless.
11. Execution policy `Restricted` machine: the one-liner still works (`irm | iex` is not a script file; the task uses `-ExecutionPolicy Bypass`).

## After either

- Revoke the Host from its detail page: the machine stops uploading (watcher logs 401) and the Host shows Revoked.
- Record OS version, architecture and result for each step in the release notes.
