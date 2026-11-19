#!/usr/bin/env bash
# Installs agentsview and sends this login's agent sessions to __PLATFORM_URL__.
# Usage: curl -fsSL __PLATFORM_URL__/install/macos.sh | AVT_TOKEN=<token> bash
# Env: AVT_TOKEN (else prompted on /dev/tty), AV_HOST_NAME (default: computer name).
# Test seams: AV_PLATFORM_URL, AV_DOWNLOAD_BASE, AV_SKIP_LAUNCHCTL=1 (write files, do not load the agent).
# Installs a launchd user agent; needs no root. Safe to re-run: it upgrades in place and keeps the Host's existing Credential.
set -euo pipefail

platform="${AV_PLATFORM_URL:-__PLATFORM_URL__}"
version="__VERSION__"
download="${AV_DOWNLOAD_BASE:-https://github.com/kenn-io/agentsview/releases/download}"

[ "$(uname -s)" = Darwin ] || { echo "This script is for macOS." >&2; exit 1; }
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
for cmd in curl tar shasum; do
  command -v "$cmd" >/dev/null || { echo "$cmd is required" >&2; exit 1; }
done

bin="$HOME/.local/bin"
cfg="$HOME/.config/agentsview-platform"
share="$HOME/.local/share/agentsview-platform"
label=net.mintopia.agentsview-host
plist="$HOME/Library/LaunchAgents/$label.plist"
mkdir -p "$bin" "$cfg" "$share" "$HOME/Library/LaunchAgents"
chmod 700 "$cfg" "$share"

install_binary() {
  local tmp archive want got
  tmp=$(mktemp -d)
  archive="agentsview_${version}_darwin_${arch}.tar.gz"
  curl -fsSL "$download/v$version/$archive" -o "$tmp/$archive"
  curl -fsSL "$download/v$version/SHA256SUMS" -o "$tmp/SHA256SUMS"
  want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/SHA256SUMS")
  got=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    rm -rf "$tmp"
    echo "checksum mismatch for $archive" >&2
    exit 1
  fi
  tar -xzf "$tmp/$archive" -C "$tmp" agentsview
  # Ad-hoc signing, as upstream's installer does: unsigned arm64 binaries are killed.
  codesign -s - --force "$tmp/agentsview" >/dev/null 2>&1 || true
  install -m 755 "$tmp/agentsview" "$bin/agentsview.new"
  mv -f "$bin/agentsview.new" "$bin/agentsview"
  rm -rf "$tmp"
}

case "$("$bin/agentsview" --version 2>/dev/null || true)" in
  "agentsview v$version "*) ;;
  *) install_binary ;;
esac

if [ ! -f "$cfg/host.env" ]; then
  token="${AVT_TOKEN:-}"
  if [ -z "$token" ] && [ -r /dev/tty ]; then
    read -rp "Install Token: " token </dev/tty
  fi
  [[ "$token" =~ ^avt_tk_[A-Za-z0-9_-]+$ ]] || { echo "Set AVT_TOKEN to the Install Token from Add Host." >&2; exit 1; }
  name=$(printf '%s' "${AV_HOST_NAME:-$(scutil --get ComputerName 2>/dev/null || hostname -s)}" | tr -c 'A-Za-z0-9._-' '-' | cut -c1-64)
  os=$(printf 'macOS %s' "$(sw_vers -productVersion 2>/dev/null)" | tr -c 'A-Za-z0-9 ._()/-' '-' | sed 's/ *$//' | cut -c1-64)
  reply=$(mktemp)
  code=$(printf '{"token":"%s","name":"%s","os":"%s","mode":"launchd","version":"%s"}' "$token" "$name" "$os" "$version" |
    curl -sS -o "$reply" -w '%{http_code}' -H 'Content-Type: application/json' -H 'Accept: text/x-env' \
      --data-binary @- "$platform/api/hosts/enrol") || code=000
  if [ "$code" != 200 ]; then
    echo "Could not add this Host (HTTP $code): $(cat "$reply")" >&2
    rm -f "$reply"
    exit 1
  fi
  if grep -qvE '^[A-Z_]+=[A-Za-z0-9._:/-]*$' "$reply"; then
    echo "unexpected reply from $platform" >&2
    rm -f "$reply"
    exit 1
  fi
  user_path=$(sed -n 's/^AV_USER_PATH=//p' "$reply")
  (
    umask 077
    {
      grep -v '^AV_USER_PATH=' "$reply"
      echo "AGENTSVIEW_RAW_SYNC_URL=$platform$user_path"
      echo "AV_INSTALL_MODE=launchd"
    } >"$cfg/host.env.tmp"
    mv "$cfg/host.env.tmp" "$cfg/host.env"
  )
  rm -f "$reply"
fi

# macOS ships bash 3.2: empty arrays trip set -u, hence the ${flags[@]+...} form.
cat >"$share/run.sh" <<'RUN'
#!/bin/bash
set -u
base="$HOME/.local/share/agentsview-platform"
set -a
# shellcheck disable=SC1090
. "$HOME/.config/agentsview-platform/host.env"
set +a
export AGENTSVIEW_DATA_DIR="$base/data" AGENTSVIEW_TELEMETRY_ENABLED=0
export PATH="$HOME/.local/bin:$PATH"
mkdir -p "$AGENTSVIEW_DATA_DIR"
# The log is appended to (a refused second start must not truncate it), so trim it here.
[ "$(wc -c <"$base/agent.log" 2>/dev/null || echo 0)" -lt 1048576 ] || : >"$base/agent.log"
flags=()
case "$AGENTSVIEW_RAW_SYNC_URL" in http://*) flags=(--allow-insecure-http) ;; esac

# Upload existing history once. Backfill refuses a provider with any missing root, so each
# provider that exists here gets its own run. Exit 2 means incomplete, so run it again.
ran="$base/backfilled.d"
mkdir -p "$ran"
say() { echo "$(date -u +%FT%TZ) $*"; }
backfill_run() { # run-id provider [ENV=path]; 0 done, 3 a root is missing, 1 failed
  local id=$1 prov=$2 out rc
  [ -f "$ran/$id" ] && return 0
  say "backfill $id: starting"
  while :; do
    out=$(env ${3:+"$3"} agentsview source raw backfill --run-id "$id" --provider "$prov" --format json ${flags[@]+"${flags[@]}"} 2>&1)
    rc=$?
    printf '%s\n' "$out" >>"$base/backfill.log"
    [ "$rc" -eq 2 ] || break
  done
  if [ "$rc" -ne 0 ]; then
    case $out in
      *"does not support raw capture"*) say "backfill $id: skipped, $prov cannot upload raw sessions" ;;
      *"root is unavailable"*)
        say "backfill $id: a $prov folder is missing, backfilling each folder separately"
        return 3 ;;
      *)
        say "backfill $id: failed, retrying on the next start (details in backfill.log)"
        return 1 ;;
    esac
  else
    say "backfill $id: done, $(printf '%s' "$out" | sed -n 's/.*"captured":\([0-9]*\).*/\1/p' | tail -n 1) sessions"
  fi
  touch "$ran/$id"
}

# Marker 2: an older run.sh wrote an empty marker after backfilling only claude and codex.
if [ "$(cat "$base/backfilled" 2>/dev/null)" != 2 ]; then
  all=1
  listing=$(agentsview source list --format json 2>>"$base/backfill.log") || all=0
  # One line per provider with default roots: name|ENV_VAR|root|root...
  rows=$(printf '%s\n' "$listing" | awk '
    /^ +"type":/ { gsub(/[",]/, "", $2); t = $2; skip = 0; dirs = ""; env = "" }
    /"import_only": true/ { skip = 1 }
    /"default_dirs": \[/ { d = 1; next }
    d && /\]/ { d = 0 }
    d { gsub(/^ *"|",? *$/, ""); dirs = dirs "|" $0 }
    /"env_var":/ { gsub(/[",]/, "", $2); env = $2 }
    /"content":/ && !skip && dirs != "" { print t "|" env dirs }')
  while IFS='|' read -r -a row; do
    prov=${row[0]:-}
    [ -n "$prov" ] || continue
    present=()
    for rel in "${row[@]:2}"; do [ -e "$HOME/$rel" ] && present+=("$HOME/$rel"); done
    [ "${#present[@]}" -gt 0 ] || continue
    backfill_run "initial-$prov" "$prov"
    case $? in
      1) all=0 ;;
      3)
        n=0
        if [ -n "${row[1]}" ]; then
          for dir in "${present[@]}"; do
            n=$((n + 1))
            backfill_run "initial-$prov-$n" "$prov" "${row[1]}=$dir"
            [ $? -ne 1 ] || all=0
          done
        fi ;;
    esac
  done <<<"$rows"
  if [ "$all" -eq 1 ]; then
    echo 2 >"$base/backfilled"
    say "initial backfill complete"
  else
    say "initial backfill incomplete, retrying on the next start"
  fi
fi

say "starting watcher for $AGENTSVIEW_RAW_SYNC_URL"
agentsview source raw watch ${flags[@]+"${flags[@]}"} &
watch=$!
trap 'kill "$watch" 2>/dev/null; exit 0' TERM INT
# An idle watcher sends nothing; server-status keeps the Host showing as online.
while kill -0 "$watch" 2>/dev/null; do
  agentsview source raw server-status ${flags[@]+"${flags[@]}"} >/dev/null 2>&1 || true
  ver=$(agentsview --version 2>/dev/null | awk '{ sub(/^v/, "", $2); print $2; exit }')
  # The Credential goes in on stdin so it never shows in the process list.
  [ -z "$ver" ] || printf 'header = "Authorization: Bearer %s"\n' "$AGENTSVIEW_RAW_SYNC_CREDENTIAL" |
    curl -fsS -m 20 -o /dev/null -X POST -K - -H 'Content-Type: application/json' \
      -H "X-AgentsView-Device-ID: $AGENTSVIEW_RAW_SYNC_DEVICE_ID" \
      -d "{\"version\":\"$ver\"}" "${AGENTSVIEW_RAW_SYNC_URL%/i/*}/api/hosts/checkin" >/dev/null 2>&1 || true
  sleep 600 &
  wait $!
done
wait "$watch"
RUN
chmod 755 "$share/run.sh"

cat >"$plist.tmp" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key><array><string>$share/run.sh</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>$share/agent.log</string>
  <key>StandardErrorPath</key><string>$share/agent.log</string>
</dict>
</plist>
EOF
mv -f "$plist.tmp" "$plist"

if [ "${AV_SKIP_LAUNCHCTL:-0}" = 1 ]; then
  echo "Wrote $plist (agent not loaded)."
  exit 0
fi
domain="gui/$(id -u)"
launchctl bootout "$domain/$label" >/dev/null 2>&1 || true
if launchctl bootstrap "$domain" "$plist" 2>/dev/null; then
  echo "Installed as the launchd user agent $label."
else
  echo "Installed, but launchd has no login session here (SSH?). The agent starts at your next login."
fi
echo "Done. Sessions from this machine will appear in your Instance."
