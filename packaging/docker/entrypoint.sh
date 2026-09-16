#!/bin/bash
set -euo pipefail
umask 022
mkdir -p /run/dbus /run/avahi-daemon
rm -f /run/dbus/pid /run/dbus/system_bus_socket /run/avahi-daemon/pid
dbus-uuidgen --ensure
pids=()
cleanup() {
  trap - EXIT
  trap '' TERM INT
  kill -TERM "${pids[@]}" 2>/dev/null || true
  wait || true
}
trap 'cleanup; exit 0' TERM INT
trap cleanup EXIT
dbus-daemon --system --nofork &
pids+=("$!")
for attempt in {1..50}; do
  dbus-send --system --type=method_call --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames >/dev/null 2>&1 && break
  kill -0 "${pids[0]}"
  sleep 0.1
done
dbus-send --system --type=method_call --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames >/dev/null
avahi-daemon --no-chroot &
pids+=("$!")
for attempt in {1..50}; do
  avahi-browse --all --terminate >/dev/null 2>&1 && break
  kill -0 "${pids[1]}"
  sleep 0.1
done
avahi-browse --all --terminate >/dev/null
umask 077
"$@" &
app_pid=$!
pids+=("$app_pid")
status=0
wait -n -p exited "${pids[@]}" || status=$?
if [ "$exited" != "$app_pid" ]; then
  echo "Container D-Bus or Avahi exited unexpectedly" >&2
  exit 1
fi
exit "$status"
