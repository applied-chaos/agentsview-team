#!/bin/sh
# Renders mockups at desktop (1440x900) and mobile (390x844), light and dark, into png/.
# A "full" query renders desktop 1500px tall (for content below the fold).
# Usage: ./shoot.sh [page[?query][#state] ...], e.g. ./shoot.sh hosts 'instance?banner=view' 'hosts#dlg-token'
# Headless Chrome will not go below 500px wide, so mobile renders inside a 390px iframe and is cropped.
set -eu
cd "$(dirname "$0")"
chrome="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
themes="${THEMES:-light dark}"
shot() { "$chrome" --headless=new --disable-gpu --hide-scrollbars --force-color-profile=srgb \
  --virtual-time-budget=4000 --window-size="$1" --screenshot="$2" "$3" 2>/dev/null; }
mkdir -p png
all="sign-in not-permitted error?reason=sign-in-expired error?reason=no-verified-email error?reason=not-found overview overview#settings overview?menu=open overview-empty overview-loading hosts-loading hosts hosts#dlg-token hosts#dlg-created
  add-host add-host#connected host host#dlg-revoke host#dlg-delete team instance instance?banner=view instance?banner=imp instance-failed
  admin-users admin-users#dlg-grant admin-user admin-user#dlg-disable admin-user#dlg-delete admin-instance audit"
pages="${*:-$all}"
for p in $pages; do
  file=$(echo "$p" | sed 's/[?#].*//')
  query=$(echo "$p" | sed -n 's/^[^?#]*?\([^#]*\).*/\1/p')
  state=$(echo "$p" | sed -n 's/^[^#]*\(#.*\)/\1/p')
  for t in $themes; do
    url="$file.html?theme=$t${query:+&$query}$state"
    name=$(echo "$p" | tr '?#=&' '----')-$t
    case "$query" in *full*) h=1500;; *) h=900;; esac
    shot 1440,$h "png/$name-desktop.png" "file://$PWD/$url"
    shot 500,844 "png/$name-mobile.png" "file://$PWD/frame.html#$(echo "$url" | sed 's/?/%3F/; s/#/%23/; s/&/%26/g; s/=/%3D/g')"
    ffmpeg -loglevel error -y -i "png/$name-mobile.png" -vf crop=390:844:0:0 "png/$name-m.png" && mv "png/$name-m.png" "png/$name-mobile.png"
  done
done
