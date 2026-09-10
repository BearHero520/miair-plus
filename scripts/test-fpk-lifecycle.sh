#!/bin/sh
# Run as root in Linux CI: reproduce private root-owned state from older FPKs.
set -eu
[ "$(id -u)" = 0 ]
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
CASE=$(mktemp -d /tmp/miair-lifecycle.XXXXXX)
chmod 755 "$CASE"
export TRIM_APPDEST="$CASE/app" TRIM_PKGVAR="$CASE/state"
export TRIM_USERNAME=nobody TRIM_GROUPNAME=$(id -gn nobody)
export TRIM_TEMP_LOGFILE="$CASE/error.txt"
cleanup() {
  sh "$ROOT/packaging/fpk/cmd/main" stop || :
  rm -rf "$CASE"
}
trap cleanup EXIT
mkdir -p "$TRIM_APPDEST/bin/amd64" "$TRIM_PKGVAR/data"
cp "$ROOT/build/miair-plus-linux-amd64" "$TRIM_APPDEST/bin/amd64/miair-plus"
printf '{}\n' > "$TRIM_PKGVAR/data/miair-plus.json"
chmod 700 "$TRIM_PKGVAR/data"
chmod 600 "$TRIM_PKGVAR/data/miair-plus.json"
printf '99999999\n' > "$TRIM_PKGVAR/app.pid"
chmod 600 "$TRIM_PKGVAR/app.pid"
sh "$ROOT/packaging/fpk/cmd/main" start
sh "$ROOT/packaging/fpk/cmd/main" status
[ "$(stat -c %U "$TRIM_PKGVAR/data/miair-plus.json")" = nobody ]
[ "$(stat -c %a "$TRIM_PKGVAR/data/miair-plus.json")" = 600 ]
curl -fsS http://127.0.0.1:8310/ >/dev/null
sh "$ROOT/packaging/fpk/cmd/main" stop
sh "$ROOT/packaging/fpk/cmd/main" start
sh "$ROOT/packaging/fpk/cmd/main" stop
mv "$TRIM_APPDEST/bin/amd64/miair-plus" "$CASE/saved-binary"
if sh "$ROOT/packaging/fpk/cmd/main" start; then
  echo 'Missing executable incorrectly started' >&2
  exit 1
fi
grep -q 'binary missing' "$TRIM_PKGVAR/startup.log"
grep -q 'startup.log' "$TRIM_TEMP_LOGFILE"
echo 'FPK legacy permissions, restart and startup failure reporting passed'
