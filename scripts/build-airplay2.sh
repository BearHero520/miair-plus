#!/bin/bash
# Runs only in a disposable Debian 12 build container, never on the NAS.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends build-essential git ca-certificates autoconf automake libtool pkg-config nasm curl xz-utils libpopt-dev libconfig-dev libavahi-client-dev libssl-dev libsoxr-dev libplist-dev libsodium-dev uuid-dev libgcrypt20-dev xxd libplist-utils libmp3lame-dev libcap2-bin
mkdir -p /build /out/bin /out/lib /out/licenses /out/sources
cd /build
git clone --depth 1 --branch n8.1 https://github.com/FFmpeg/FFmpeg.git ffmpeg
cd ffmpeg
./configure --prefix=/opt/miair-audio --disable-everything --disable-autodetect --disable-doc --disable-debug --disable-ffplay --enable-small --enable-shared --disable-static --enable-gpl --enable-version3 --enable-openssl --enable-libmp3lame --enable-ffmpeg --enable-ffprobe --enable-avcodec --enable-avformat --enable-avutil --enable-swresample --enable-avfilter --enable-protocol=file,pipe,http,https,tcp,tls,crypto,udp --enable-demuxer=pcm_s16be,pcm_s16le,mov,mp3,aac,flac,wav,ogg --enable-muxer=mp3,mp4,ipod,adts,wav,pcm_s16le --enable-parser=aac,mpegaudio,flac,opus,vorbis --enable-decoder=alac,aac,mp3,flac,pcm_s16le,pcm_s16be,pcm_s24le,pcm_s32le,vorbis,opus --enable-encoder=libmp3lame,pcm_s16le,alac --enable-filter=aresample,aformat,anull,volume,asetpts,atrim,sine --enable-indev=lavfi --enable-bsf=aac_adtstoasc
make -j"$(nproc)"
make install
git rev-parse HEAD > /out/licenses/ffmpeg.commit
git archive --format=tar HEAD | xz -T0 > /out/sources/ffmpeg-source.tar.xz
cp COPYING.GPLv3 /out/licenses/FFmpeg-COPYING
export PKG_CONFIG_PATH=/opt/miair-audio/lib/pkgconfig
export LD_LIBRARY_PATH=/opt/miair-audio/lib
cd /build
git clone --depth 1 --branch 5.5.1 https://github.com/mikebrady/shairport-sync.git
cd shairport-sync
# 5.5.1 overwrites an explicitly configured RTSP port during startup.
git apply /workspace/scripts/patches/shairport-sync-custom-port.patch
cp /workspace/scripts/patches/shairport-sync-custom-port.patch /out/licenses/
cp /workspace/scripts/patches/shairport-sync-custom-port.patch /out/sources/
autoreconf -fi
./configure --prefix=/opt/miair-airplay2 --sysconfdir=/etc --with-stdout --with-avahi --with-ssl=openssl --with-airplay-2 --with-soxr --with-metadata --with-metadata-multicast
make -j"$(nproc)"
cp shairport-sync /out/bin/shairport-sync.real
git rev-parse HEAD > /out/licenses/shairport-sync.commit
git archive --format=tar HEAD | xz -T0 > /out/sources/shairport-sync-source.tar.xz
cp COPYING /out/licenses/Shairport-Sync-COPYING
cd /build
git clone --depth 1 --branch 1.2.8 https://github.com/mikebrady/nqptp.git
cd nqptp
autoreconf -fi
./configure --prefix=/opt/miair-airplay2
make -j"$(nproc)"
cp nqptp /out/bin/nqptp
git rev-parse HEAD > /out/licenses/nqptp.commit
git archive --format=tar HEAD | xz -T0 > /out/sources/nqptp-source.tar.xz
cp COPYING /out/licenses/NQPTP-COPYING
cp /opt/miair-audio/bin/ffmpeg /out/bin/ffmpeg.real
cp /opt/miair-audio/bin/ffprobe /out/bin/ffprobe.real
cp -L /opt/miair-audio/lib/*.so.* /out/lib/
# ldd includes the transitive dependencies. Keep fnOS's libc/loader; all bundled
# binaries are built against Debian 12 glibc for fnOS compatibility.
ldd /out/bin/*.real /out/bin/nqptp | awk '/=> \// {print $3}' | sort -u > /build/libraries
while IFS= read -r lib; do
  case "$(basename "$lib")" in libc.so.*|libm.so.*|libpthread.so.*|librt.so.*|libdl.so.*) continue ;; esac
  cp -L "$lib" /out/lib/
  if [[ "$lib" != /opt/miair-audio/* ]]; then
    package=$( { dpkg-query -S "$lib" 2>/dev/null || dpkg-query -S "$(readlink -f "$lib")" 2>/dev/null || dpkg-query -S "${lib#/usr}" 2>/dev/null; } | sed -n '1s/: \/.*//p')
    if [[ -n "$package" ]]; then dpkg-query -W -f='${source:Package}=${source:Version}\n' "$package" >> /out/licenses/debian-source-versions.txt; fi
  fi
done < /build/libraries
sort -u -o /out/licenses/debian-source-versions.txt /out/licenses/debian-source-versions.txt
cat > /etc/apt/sources.list.d/miair-sources.list <<'EOF'
deb-src [signed-by=/usr/share/keyrings/debian-archive-keyring.gpg] http://deb.debian.org/debian bookworm main
deb-src [signed-by=/usr/share/keyrings/debian-archive-keyring.gpg] http://deb.debian.org/debian bookworm-updates main
deb-src [signed-by=/usr/share/keyrings/debian-archive-keyring.gpg] http://security.debian.org/debian-security bookworm-security main
EOF
apt-get update
cd /out/sources
while IFS= read -r source; do apt-get source --download-only "$source"; done < /out/licenses/debian-source-versions.txt
for program in ffmpeg ffprobe shairport-sync; do
  cat > "/out/bin/$program" <<'EOF'
#!/bin/sh
set -eu
base=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export LD_LIBRARY_PATH="$base/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
exec "$base/bin/$(basename "$0").real" "$@"
EOF
done
chmod 755 /out/bin/*
strip --strip-unneeded /out/bin/*.real /out/bin/nqptp /out/lib/*.so.*
/out/bin/ffmpeg -version
/out/bin/shairport-sync -V
/out/bin/nqptp -V
cp /workspace/scripts/build-airplay2.sh /out/licenses/build-airplay2.sh
cd /out
find bin lib licenses -type f -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
