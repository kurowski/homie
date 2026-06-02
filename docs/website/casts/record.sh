#!/usr/bin/env bash
# Render homie screencasts with VHS, fully containerized so a local run
# produces the same artifacts as CI (see PLAN.md).
#
# For the bootstrap hero it reuses the e2e harness end-to-end:
#   1. build hm (also the binary the hero "downloads")
#   2. build the pinned recorder image (VHS stack + a fresh Linux box)
#   3. stand up an nginx sidecar impersonating github.com /
#      raw.githubusercontent.com over the committed e2e test CA, serving a
#      freshly `hm init`-scaffolded demo repo + the hm release artifacts
#   4. run vhs inside a container on that network, so the in-cast
#      `curl … | bash` is the real bootstrap, offline and deterministic
#   5. copy the rendered .gif/.webm into static/casts/
#
# Usage: ./record.sh [tape ...]   (default: every *.tape in this dir)
set -euo pipefail

CASTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEBSITE_DIR="$(cd "$CASTS_DIR/.." && pwd)"
REPO_ROOT="$(cd "$WEBSITE_DIR/../.." && pwd)"
OUT_DIR="$WEBSITE_DIR/static/casts"

RECORDER_IMAGE="homie-cast-recorder"
NET="homie-cast-net"
NGINX="homie-cast-nginx"

OWNER="scouthomes"; REPO="dotfiles"
HM_OWNER="kurowski"; HM_NAME="homie"
ARCH="amd64"

tapes=("$@")
if [ "${#tapes[@]}" -eq 0 ]; then
  tapes=()
  for t in "$CASTS_DIR"/*.tape; do tapes+=("$(basename "$t")"); done
fi

WORK=""
cleanup() {
  docker rm -f "$NGINX" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  [ -n "$WORK" ] && rm -rf "$WORK"
}
trap cleanup EXIT

echo "==> building hm"
make -C "$REPO_ROOT" build >/dev/null

echo "==> building recorder image"
docker build -t "$RECORDER_IMAGE" -f "$CASTS_DIR/Dockerfile" "$REPO_ROOT/e2e/certs" >/dev/null

echo "==> scaffolding demo repo + release artifacts"
WORK="$(mktemp -d)"
SRC="$WORK/userrepo-src"
"$REPO_ROOT/hm" init </dev/null \
  --name "Scout Homes" --email "scout@homie.sh" \
  --github-user "$OWNER" --github-repo "$REPO" \
  --profile personal --shell bash "$SRC" >/dev/null

git -C "$SRC" -c init.defaultBranch=main init -q
git -C "$SRC" -c user.name="Scout Homes" -c user.email="scout@homie.sh" add -A
git -C "$SRC" -c user.name="Scout Homes" -c user.email="scout@homie.sh" commit -qm initial

BARE="$WORK/content/github/$OWNER/$REPO.git"
mkdir -p "$(dirname "$BARE")"
git clone -q --bare "$SRC" "$BARE"
git -C "$BARE" update-server-info

REL="$WORK/content/github/$HM_OWNER/$HM_NAME/releases/latest/download"
mkdir -p "$REL"
cp "$REPO_ROOT/hm" "$REL/hm-linux-$ARCH"
( cd "$REL" && sha256sum "hm-linux-$ARCH" > SHA256SUMS )

RAW="$WORK/content/raw/$OWNER/$REPO/main"
mkdir -p "$RAW"
cp "$SRC/bootstrap.sh" "$RAW/bootstrap.sh"

echo "==> starting nginx sidecar"
docker network create "$NET" >/dev/null 2>&1 || true
docker rm -f "$NGINX" >/dev/null 2>&1 || true
docker run -d --rm --name "$NGINX" --network "$NET" \
  --network-alias github.com --network-alias raw.githubusercontent.com \
  -v "$CASTS_DIR/nginx.conf:/etc/nginx/nginx.conf:ro" \
  -v "$REPO_ROOT/e2e/certs:/etc/nginx/certs:ro" \
  -v "$WORK/content/github:/var/www/github:ro" \
  -v "$WORK/content/raw:/var/www/raw:ro" \
  nginx:alpine >/dev/null

# Render into a tmp dir the in-container (non-root scout) user can write,
# then copy into the site as the invoking user — no root/scout-owned files
# land in the repo.
RENDER_OUT="$WORK/out"
mkdir -p "$RENDER_OUT"
chmod 777 "$RENDER_OUT"
for name in "${tapes[@]}"; do
  echo "==> rendering $name"
  docker run --rm --network "$NET" --security-opt seccomp=unconfined \
    -v "$CASTS_DIR:/tapes:ro" -v "$RENDER_OUT:/out" -w /out \
    "$RECORDER_IMAGE" "/tapes/$name"
done

mkdir -p "$OUT_DIR"
find "$RENDER_OUT" -maxdepth 1 -type f \( -name '*.gif' -o -name '*.webm' \) \
  -exec cp -f {} "$OUT_DIR"/ \;

echo "==> done"
ls -la "$OUT_DIR"
