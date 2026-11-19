#!/usr/bin/env bash
# Installs agentsview and sends this login's agent sessions to __PLATFORM_URL__.
# Usage: curl -fsSL __PLATFORM_URL__/install/linux.sh | AVT_TOKEN=<token> bash [-s -- --profile]
#   --profile    run one background watcher from your shell profiles instead of a systemd user service
# Env: AVT_TOKEN (else prompted on /dev/tty), AV_HOST_NAME (default: hostname).
# Needs no root. Safe to re-run: it upgrades in place and keeps the Host's existing Credential.
set -euo pipefail

# AV_PLATFORM_URL and AV_DOWNLOAD_BASE exist for tests and mirrors.
platform="${AV_PLATFORM_URL:-__PLATFORM_URL__}"
version="__VERSION__"
download="${AV_DOWNLOAD_BASE:-https://github.com/kenn-io/agentsview/releases/download}"

force_profile=0
for arg in "$@"; do
  case "$arg" in
    --profile) force_profile=1 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

[ "$(uname -s)" = Linux ] || { echo "This script is for Linux." >&2; exit 1; }
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
for cmd in curl tar sha256sum; do
  command -v "$cmd" >/dev/null || { echo "$cmd is required" >&2; exit 1; }
done

bin="$HOME/.local/bin"
cfg="${XDG_CONFIG_HOME:-$HOME/.config}/agentsview-platform"
share="${XDG_DATA_HOME:-$HOME/.local/share}/agentsview-platform"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
unit=agentsview-host.service
mkdir -p "$bin" "$cfg" "$share"
chmod 700 "$cfg" "$share"

if [ "$force_profile" = 0 ] && systemctl --user show-environment >/dev/null 2>&1; then
  mode=systemd
else
  mode=profile
fi

install_binary() {
  local archive want got
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  archive="agentsview_${version}_linux_${arch}.tar.gz"
  curl -fsSL "$download/v$version/$archive" -o "$tmp/$archive"
  curl -fsSL "$download/v$version/SHA256SUMS" -o "$tmp/SHA256SUMS"
  want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/SHA256SUMS")
  got=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
  if [ -z "$want" ] || [ "$want" != "$got" ]; then
    echo "checksum mismatch for $archive" >&2
    exit 1
  fi
  tar -xzf "$tmp/$archive" -C "$tmp" agentsview
  install -m 755 "$tmp/agentsview" "$bin/agentsview.new"
  mv -f "$bin/agentsview.new" "$bin/agentsview"
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
  name=$(printf '%s' "${AV_HOST_NAME:-$(hostname -s 2>/dev/null || hostname)}" | tr -c 'A-Za-z0-9._-' '-' | cut -c1-64)
  os=Linux
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    os=$(. /etc/os-release && printf '%s %s' "${NAME:-Linux}" "${VERSION_ID:-}")
  fi
  os=$(printf '%s' "$os" | tr -c 'A-Za-z0-9 ._()/-' '-' | sed 's/ *$//' | cut -c1-64)
  reply=$(mktemp)
  code=$(printf '{"token":"%s","name":"%s","os":"%s","mode":"%s","version":"%s"}' "$token" "$name" "$os" "$mode" "$version" |
    curl -sS -o "$reply" -w '%{http_code}' -H 'Content-Type: application/json' -H 'Accept: text/x-env' \
      --data-binary @- "$platform/api/hosts/enrol") || code=000
  if [ "$code" != 200 ]; then
    echo "Could not add this Host (HTTP $code): $(cat "$reply")" >&2
    rm -f "$reply"
    exit 1
  fi
  # Every line must be a plain KEY=value pair before it is sourced later.
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
      echo "AV_INSTALL_MODE=$mode"
    } >"$cfg/host.env.tmp"
    mv "$cfg/host.env.tmp" "$cfg/host.env"
  )
  rm -f "$reply"
fi

cat >"$share/run.sh" <<'RUN'
#!/usr/bin/env bash
set -u
base="${XDG_DATA_HOME:-$HOME/.local/share}/agentsview-platform"
set -a
# shellcheck disable=SC1090
. "${XDG_CONFIG_HOME:-$HOME/.config}/agentsview-platform/host.env"
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
agentsview source raw watch "${flags[@]}" &
watch=$!
nap=$watch
# The sleep must die too: it would keep the profile-mode flock held and block the next start.
trap 'kill "$watch" "$nap" 2>/dev/null; exit 0' TERM INT
# An idle watcher sends nothing; server-status keeps the Host showing as online.
while kill -0 "$watch" 2>/dev/null; do
  agentsview source raw server-status "${flags[@]}" >/dev/null 2>&1 || true
  ver=$(agentsview --version 2>/dev/null | awk '{ sub(/^v/, "", $2); print $2; exit }')
  # The Credential goes in on stdin so it never shows in the process list.
  [ -z "$ver" ] || printf 'header = "Authorization: Bearer %s"\n' "$AGENTSVIEW_RAW_SYNC_CREDENTIAL" |
    curl -fsS -m 20 -o /dev/null -X POST -K - -H 'Content-Type: application/json' \
      -H "X-AgentsView-Device-ID: $AGENTSVIEW_RAW_SYNC_DEVICE_ID" \
      -d "{\"version\":\"$ver\"}" "${AGENTSVIEW_RAW_SYNC_URL%/i/*}/api/hosts/checkin" >/dev/null 2>&1 || true
  sleep 600 &
  nap=$!
  wait "$nap"
done
wait "$watch"
RUN
chmod 755 "$share/run.sh"

begin='# >>> agentsview host >>>'
end='# <<< agentsview host <<<'
profiles=("$HOME/.profile" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc")

remove_profile_hook() {
  local rc
  for rc in "${profiles[@]}"; do
    [ -f "$rc" ] || continue
    awk -v b="$begin" -v e="$end" '$0 == b { skip = 1 } !skip; $0 == e { skip = 0 }' "$rc" >"$rc.tmp"
    cat "$rc.tmp" >"$rc" && rm "$rc.tmp"
  done
}

stop_watcher() {
  command -v pkill >/dev/null || return 0
  pkill -f "$share/run.sh" || true
  for _ in $(seq 1 20); do
    pgrep -f "$share/run.sh" >/dev/null || return 0
    sleep 0.5
  done
}

if [ "$mode" = systemd ]; then
  remove_profile_hook
  stop_watcher
  mkdir -p "$unit_dir"
  cat >"$unit_dir/$unit" <<EOF
[Unit]
Description=agentsview Host: uploads agent sessions to __PLATFORM_URL__
After=network-online.target

[Service]
ExecStart=$share/run.sh
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload
  systemctl --user enable "$unit" >/dev/null 2>&1
  systemctl --user restart "$unit"
  echo "Installed as the systemd user service $unit."
  if command -v loginctl >/dev/null && ! loginctl show-user "${USER:-$(id -un)}" -p Linger 2>/dev/null | grep -q 'Linger=yes'; then
    echo "To keep uploading after you log out, run: sudo loginctl enable-linger ${USER:-$(id -un)}"
  fi
else
  command -v flock >/dev/null || { echo "flock (util-linux) is required for profile mode" >&2; exit 1; }
  if [ -f "$unit_dir/$unit" ]; then
    systemctl --user disable --now "$unit" >/dev/null 2>&1 || true
    rm -f "$unit_dir/$unit"
  fi
  printf -v start '(nohup flock -n %q %q >>%q 2>&1 </dev/null &)' "$share/watch.lock" "$share/run.sh" "$share/agent.log"
  remove_profile_hook
  for rc in "${profiles[@]}"; do
    case "$rc" in */.profile | */.bashrc) touch "$rc" ;; esac
    [ -f "$rc" ] || continue
    printf '%s\n%s\n%s\n' "$begin" "$start" "$end" >>"$rc"
  done
  stop_watcher
  # A stopping watcher can hold the lock a moment longer; flock -n would then skip this start.
  flock -w 15 "$share/watch.lock" true || true
  eval "$start"
  echo "No service manager in use: a watcher is running and starts again from your shell profiles."
fi
echo "Done. Sessions from this machine will appear in your Instance."
