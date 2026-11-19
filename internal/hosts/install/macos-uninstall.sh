#!/usr/bin/env bash
# Removes the agentsview Host from this Mac: stops the launchd user agent and deletes its files.
# Usage: curl -fsSL __PLATFORM_URL__/install/macos-uninstall.sh | bash
# Sessions already uploaded stay in your Instance. Revoke the Host in the Platform to invalidate its Credential.
set -euo pipefail

label=net.mintopia.agentsview-host
plist="$HOME/Library/LaunchAgents/$label.plist"

if [ "${AV_SKIP_LAUNCHCTL:-0}" != 1 ]; then
  launchctl bootout "gui/$(id -u)/$label" >/dev/null 2>&1 || true
fi
rm -f "$plist" "$HOME/.local/bin/agentsview"
rm -rf "$HOME/.config/agentsview-platform" "$HOME/.local/share/agentsview-platform"
echo "agentsview removed from this Mac."
