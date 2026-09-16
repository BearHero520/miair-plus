# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run typecheck && npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.1-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=frontend /src/frontend/dist/ ./internal/web/dist/
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/miair-plus ./cmd/miair-plus

# Reuse the verified, custom-port-patched native runtime from the public FPK.
# Corresponding sources are attached to the same v2.0.9 release (see docs/DOCKER.md).
FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS audio
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*
ARG TARGETARCH
WORKDIR /tmp/package
RUN curl -fL --retry 3 -o package.fpk https://github.com/BearHero520/miair-plus/releases/download/v2.0.9/miair-plus-2.0.9-all.fpk \
    && echo '79553792c72544d0b15740f4de182a6b860ceb0193a0a19a4a123260376e4017  package.fpk' | sha256sum -c - \
    && tar -xf package.fpk app.tgz \
    && tar -xzf app.tgz "runtime/$TARGETARCH" \
    && mv "runtime/$TARGETARCH" /audio \
    && test -x /audio/bin/ffmpeg && test -x /audio/bin/shairport-sync && test -x /audio/bin/nqptp

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata dbus avahi-daemon avahi-utils curl tini \
    && rm -rf /var/lib/apt/lists/*
COPY --from=backend /out/miair-plus /usr/local/bin/miair-plus
COPY --from=audio /audio/ /opt/miair-audio/
COPY LICENSE UPSTREAM.md /usr/share/doc/miair-plus/
COPY --chmod=755 packaging/docker/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY packaging/docker/avahi-daemon.conf /etc/avahi/avahi-daemon.conf
ENV TZ=Asia/Shanghai MIAIR_AUDIO_DIR=/opt/miair-audio PATH=/opt/miair-audio/bin:$PATH
WORKDIR /data
EXPOSE 8310/tcp 8311/tcp
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8310/api/v1/login/status || exit 1
ENTRYPOINT ["/usr/bin/tini", "-g", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["miair-plus", "--data", "/data", "--listen", ":8310"]
