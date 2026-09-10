#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
(cd frontend && npm ci && npm run typecheck && npm run build)
cp -R frontend/dist/. internal/web/dist/
go test ./...
go run ./cmd/package --fnpack "${FNPACK:-fnpack}"
