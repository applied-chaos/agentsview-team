#!/usr/bin/env bash
# Removes the agentsview Host install made by install/linux.sh from this login.
# Usage: curl -fsSL <platform>/uninstall/linux.sh | bash [-s -- --purge]
#   --purge    also delete the local agentsview data (checkpoints and logs)
# Safe to re-run. It does not revoke the Host: do that on the Host page in the Platform.
set -euo pipefail

purge=0
for arg in "$@"; do
  case "$arg" in
    --purge) purge=1 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

bin="$HOME/.local/bin/agentsview"
cfg="${XDG_CONFIG_HOME:-$HOME/.config}/agentsview-platform"
share="${XDG_DATA_HOME:-$HOME/.local/share}/agentsview-platform"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
unit=agentsview-host.service

if [ -f "$unit_dir/$unit" ]; then
  systemctl --user disable --now "$unit" >/dev/null 2>&1 || true
  rm -f "$unit_dir/$unit"
  systemctl --user daemon-reload >/dev/null 2>&1 || true
fi

begin='# >>> agentsview host >>>'
end='# <<< agentsview host <<<'
for rc in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc"; do
  [ -f "$rc" ] || continue
  awk -v b="$begin" -v e="$end" '$0 == b { skip = 1 } !skip; $0 == e { skip = 0 }' "$rc" >"$rc.tmp"
  cat "$rc.tmp" >"$rc" && rm "$rc.tmp"
done

if command -v pkill >/dev/null; then
  pkill -f "$share/run.sh" || true
  pkill -u "$(id -u)" -f 'agentsview source raw (watch|server-status|backfill)' || true
  for _ in $(seq 1 20); do
    pgrep -f "$share/run.sh" >/dev/null || break
    sleep 0.5
  done
fi

rm -f "$bin" "$share/run.sh" "$share/watch.lock" "$share/backfilled"
rm -rf "$share/backfilled.d"
rm -rf "$cfg"
if [ "$purge" = 1 ]; then
  rm -rf "$share"
fi
echo "agentsview Host removed from this login."
[ "$purge" = 1 ] || echo "Local data kept in $share; add --purge to delete it."
