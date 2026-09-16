#!/bin/bash
set -euo pipefail
image=${1:-miair-plus:test}
cleanup() {
  docker logs miair-docker-test 2>&1 || true
  docker rm -f miair-docker-test 2>/dev/null || true
  docker volume rm miair-docker-test-data 2>/dev/null || true
}
trap cleanup EXIT
docker compose config --quiet
docker run --rm --entrypoint ffmpeg "$image" -version
docker run --rm --entrypoint shairport-sync "$image" -V
CGO_ENABLED=0 go test -c -o /tmp/test-airplay2 ./internal/airplay2
CGO_ENABLED=0 go test -c -o /tmp/test-airplay ./internal/airplay
docker run --rm --network host -v /tmp/test-airplay2:/test:ro -e TEST_AIRPLAY2_RUNTIME=/opt/miair-audio -e TEST_FFMPEG=/opt/miair-audio/bin/ffmpeg "$image" /test -test.v -test.timeout=90s
docker run --rm --network host -v /tmp/test-airplay:/test:ro -e TEST_FFMPEG=/opt/miair-audio/bin/ffmpeg "$image" /test -test.v -test.timeout=90s
docker run -d --name miair-docker-test --network host -v miair-docker-test-data:/data "$image"
for attempt in {1..40}; do
  if curl -fsS http://127.0.0.1:8310/api/v1/login/status >/tmp/login-status; then break; fi
  sleep 1
done
grep -q '"initialized":false' /tmp/login-status
curl -fsS http://127.0.0.1:8310/ | grep -q '<html'
curl -fsS -H 'Content-Type: application/json' -d '{"username":"docker-test","password":"Container-test-209!"}' http://127.0.0.1:8310/api/v1/login/setup | grep -q access_token
docker stop --time 30 miair-docker-test
test "$(docker inspect -f '{{.State.ExitCode}}' miair-docker-test)" = 0
docker start miair-docker-test
for attempt in {1..40}; do
  if curl -fsS http://127.0.0.1:8310/api/v1/login/status >/tmp/login-status; then break; fi
  sleep 1
done
grep -q '"initialized":true' /tmp/login-status
for attempt in {1..50}; do
  status=$(docker inspect -f '{{.State.Health.Status}}' miair-docker-test)
  [ "$status" = healthy ] && break
  sleep 1
done
test "$status" = healthy
