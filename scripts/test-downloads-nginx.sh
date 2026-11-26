#!/usr/bin/env bash
# Infrastructure test of the /downloads/ location in deploy/nginx.conf, against real Nginx in Docker.
#
#   scripts/test-downloads-nginx.sh            # nginx:1.27-alpine + deploy/nginx.conf + ./dist (run `npm run build` first)
#   scripts/test-downloads-nginx.sh <image>    # a built website image (deploy/Dockerfile)
#
# It creates a throw-away release storage with TEST data only (never production), mounts its public/ directory
# read-only exactly like the Compose file does, and checks: manifest and file headers, attachment disposition,
# byte ranges, integrity through SHA256SUMS, the allowlist, no listing, private directories, traversal,
# symlinks, methods, and that the rest of the site still works.
set -euo pipefail

IMAGE="${1:-}"
WORK="$(mktemp -d)"
NAME="apx-downloads-test-$$"
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# ---- test storage: the same layout as ansible-library's photobooth_release_storage ----
V=0.0.1
EXE="ApexVoidPhotobooth-v$V-windows-x64.exe"
S="$WORK/store"
mkdir -p "$S/public/releases/v$V" "$S/public/releases/v0.0.2" "$S/staging/v$V.abc" "$S/state/manifests"
head -c 3145728 /dev/urandom >"$S/public/releases/v$V/$EXE"
SHA="$(sha256sum "$S/public/releases/v$V/$EXE" | cut -d' ' -f1)"
SIZE="$(stat -c%s "$S/public/releases/v$V/$EXE")"
echo "$SHA  $EXE" >"$S/public/releases/v$V/SHA256SUMS"
echo "# Test notes" >"$S/public/releases/v$V/RELEASE-NOTES.md"
echo "test notices" >"$S/public/releases/v$V/THIRD-PARTY-NOTICES.txt"
echo "not allow-listed" >"$S/public/releases/v$V/other.txt"
echo "private" >"$S/staging/v$V.abc/secret.txt"
echo '{"private": true}' >"$S/state/journal.json"
ln -s /etc/passwd "$S/public/releases/v0.0.2/ApexVoidPhotobooth-v0.0.2-windows-x64.exe"
cat >"$S/public/index.json" <<EOF
{"schemaVersion": 1, "updatedAt": "2027-01-01T00:00:00Z", "latest": {"stable": "$V"}, "releases": []}
EOF
chmod -R a+rX "$S/public"
chmod 700 "$S/staging" "$S/state"

# ---- the container, as in deployment-template/docker-compose.yml.j2 ----
args=(--read-only --tmpfs /var/cache/nginx --tmpfs /var/run
  --mount "type=bind,source=$S/public,target=/srv/photobooth-downloads,readonly"
  -p 127.0.0.1::80)
if [[ -n "$IMAGE" ]]; then
  docker run -d --name "$NAME" "${args[@]}" "$IMAGE" >/dev/null
else
  [[ -f dist/index.html ]] || { echo "dist/ is missing: run npm run build first" >&2; exit 2; }
  docker run -d --name "$NAME" "${args[@]}" \
    -v "$PWD/deploy/nginx.conf:/etc/nginx/conf.d/default.conf:ro" \
    -v "$PWD/dist:/usr/share/nginx/html:ro" nginx:1.27-alpine >/dev/null
fi
PORT="$(docker port "$NAME" 80/tcp | head -1 | sed 's/.*://')"
BASE="http://127.0.0.1:$PORT"
for _ in $(seq 50); do curl -fs -o /dev/null "$BASE/" && break; sleep 0.2; done

fails=0
pass() { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1"; fails=$((fails + 1)); }
status() { curl -s -o /dev/null -w '%{http_code}' --path-as-is "$@"; }
header() { curl -s -D - -o /dev/null --path-as-is "${@:2}" | tr -d '\r' | awk -v h="$(echo "$1" | tr 'A-Z' 'a-z')" 'BEGIN{FS=": "} tolower($1)==h {sub(/^[^:]*: /,""); print; exit}'; }
expect() { # name expected actual
  if [[ "$2" == "$3" ]]; then pass "$1"; else fail "$1 (expected '$2', got '$3')"; fi
}
expect_match() { # name regex actual
  if [[ "$3" =~ $2 ]]; then pass "$1"; else fail "$1 (got '$3')"; fi
}

echo "manifest"
M="$BASE/downloads/index.json"
expect "index.json 200" 200 "$(status "$M")"
expect_match "index.json is JSON" '^application/json' "$(header content-type "$M")"
expect "index.json revalidates" "no-cache" "$(header cache-control "$M")"
expect "index.json nosniff" "nosniff" "$(header x-content-type-options "$M")"
expect "index.json not compressed" "" "$(header content-encoding -H 'Accept-Encoding: gzip' "$M")"
expect "HEAD index.json 200" 200 "$(status -I "$M")"
expect "POST index.json 405" 405 "$(status -X POST "$M")"

echo "executable"
E="$BASE/downloads/releases/v$V/$EXE"
expect "exe 200" 200 "$(status "$E")"
expect "HEAD exe 200" 200 "$(status -I "$E")"
expect "exe size" "$SIZE" "$(header content-length -I "$E")"
expect "exe type" "application/octet-stream" "$(header content-type -I "$E")"
expect "exe attachment" "attachment; filename=\"$EXE\"" "$(header content-disposition -I "$E")"
expect "exe bounded revalidation cache" "public, max-age=300, must-revalidate" "$(header cache-control -I "$E")"
expect "exe accepts ranges" "bytes" "$(header accept-ranges -I "$E")"
expect "exe range 206" 206 "$(status -r 0-99 "$E")"
expect "exe content-range" "bytes 0-99/$SIZE" "$(header content-range -r 0-99 "$E")"
expect "range bytes" "$(head -c 100 "$S/public/releases/v$V/$EXE" | sha256sum)" "$(curl -s -r 0-99 "$E" | sha256sum)"
expect "exe not compressed" "" "$(header content-encoding -H 'Accept-Encoding: gzip' "$E")"
expect "DELETE exe 405" 405 "$(status -X DELETE "$E")"

echo "integrity through SHA256SUMS"
D="$WORK/dl"
mkdir -p "$D"
curl -fs -o "$D/$EXE" "$E" && curl -fs -o "$D/SHA256SUMS" "$BASE/downloads/releases/v$V/SHA256SUMS"
if (cd "$D" && sha256sum -c --quiet SHA256SUMS); then pass "downloaded exe matches SHA256SUMS"; else fail "checksum"; fi

echo "release text files"
for f in SHA256SUMS RELEASE-NOTES.md THIRD-PARTY-NOTICES.txt; do
  expect "$f 200" 200 "$(status "$BASE/downloads/releases/v$V/$f")"
  expect "$f text" "text/plain; charset=utf-8" "$(header content-type "$BASE/downloads/releases/v$V/$f")"
done
expect "missing optional LICENSE.txt is 404" 404 "$(status "$BASE/downloads/releases/v$V/LICENSE.txt")"
expect "missing executable has no cache policy" "" "$(header cache-control -I "$BASE/downloads/releases/v0.0.2/ApexVoidPhotobooth-v0.0.2-windows-x64.exe")"
expect "missing executable has no attachment header" "" "$(header content-disposition -I "$BASE/downloads/releases/v0.0.2/ApexVoidPhotobooth-v0.0.2-windows-x64.exe")"

echo "allowlist, listing, private storage, traversal, symlinks"
for p in /downloads/ /downloads/releases/ "/downloads/releases/v$V/" "/downloads/releases/v$V" \
  "/downloads/releases/v$V/other.txt" /downloads/staging/ "/downloads/staging/v$V.abc/secret.txt" \
  /downloads/state/journal.json /downloads/../state/journal.json "/downloads/releases/v$V/../../../etc/passwd" \
  "/downloads/releases/v$V/%2e%2e/%2e%2e/%2e%2e/etc/passwd" "/downloads/releases/v$V/..%2f..%2fstate/journal.json" /downloads/index.json.bak "/downloads/releases/v01.0.0/$EXE" \
  "/downloads/releases/v$V/ApexVoidPhotobooth-v$V-windows-x64.EXE" /downloads/releases/v0.0.2/ApexVoidPhotobooth-v0.0.2-windows-x64.exe \
  /downloads/nothing.json; do
  code="$(status "$BASE$p")"
  if [[ "$code" == 404 || "$code" == 400 || "$code" == 403 ]]; then pass "$p → $code"; else fail "$p must not be served (got $code)"; fi
done
body="$(curl -s --path-as-is "$BASE/downloads/releases/v0.0.2/ApexVoidPhotobooth-v0.0.2-windows-x64.exe")"
if [[ "$body" == *"root:"* ]]; then fail "symlink followed"; else pass "symlink not followed"; fi

echo "the rest of the site"
expect "/ 200" 200 "$(status "$BASE/")"
expect "/download/ 200" 200 "$(status "$BASE/download/")"
expect "/missing 404" 404 "$(status "$BASE/missing-page/")"

echo "storage is read-only in the container"
if docker exec "$NAME" sh -c 'touch /srv/photobooth-downloads/x' 2>/dev/null; then fail "mount is writable"; else pass "mount is read-only"; fi

if ((fails)); then
  echo "$fails check(s) failed"
  docker logs "$NAME" 2>&1 | tail -20
  exit 1
fi
echo "all download checks passed"
