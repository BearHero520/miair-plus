#!/bin/bash
# Disposable CI container only. Exercise real receiver startup without root.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends dbus avahi-daemon avahi-utils libcap2-bin
mkdir -p /run/dbus
dbus-daemon --system --fork
avahi-daemon --daemonize --no-chroot
useradd --create-home miair-test
setcap cap_net_bind_service=ep /out/bin/nqptp
runuser -u miair-test -- env TEST_AIRPLAY2_RUNTIME=/out TEST_FFMPEG=/out/bin/ffmpeg /test-airplay2 -test.v -test.timeout=45s
runuser -u miair-test -- env TEST_FFMPEG=/out/bin/ffmpeg /test-airplay -test.v -test.timeout=60s
