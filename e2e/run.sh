#!/usr/bin/env bash
# End-to-end smoke test: real sshd + MinIO + Wails server-mode build + Playwright (in Docker).
# Linux only (uses --network host). Screenshots land in e2e/out.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PW_IMAGE="${PW_IMAGE:-mcr.microsoft.com/playwright:v1.65.0-noble}"
MINIO_IMAGE="${PORTUNUS_MINIO_IMAGE:-bitnamilegacy/minio:latest}"
OUT="$ROOT/e2e/out"
E2E_DIR="$(mktemp -d /tmp/portunus-e2e.XXXXXX)"
MINIO_USER=e2eadmin
MINIO_PASS=e2esecret123

free_port() {
  local p
  while :; do
    p=$((20000 + RANDOM % 20000))
    ss -ltn "sport = :$p" | grep -q LISTEN || { echo "$p"; return; }
  done
}

cleanup() {
  [[ -f "$E2E_DIR/sshd/sshd.pid" ]] && kill "$(cat "$E2E_DIR/sshd/sshd.pid")" 2>/dev/null || true
  [[ -n "${SERVER_PID:-}" ]] && kill "$SERVER_PID" 2>/dev/null || true
  [[ -n "${MINIO_ID:-}" ]] && docker rm -f "$MINIO_ID" >/dev/null 2>&1 || true
  rm -rf "$E2E_DIR"
}
trap cleanup EXIT

SSH_PORT="$(free_port)"
APP_PORT="$(free_port)"
H="$E2E_DIR/home"

echo "== fixtures in $E2E_DIR"
mkdir -p "$H/.ssh" "$E2E_DIR/config" "$E2E_DIR/remote" "$E2E_DIR/sshd" "$H/Documents/photos/2026" "$H/Documents/photos/empty"
chmod 700 "$H/.ssh"
ssh-keygen -q -t ed25519 -N '' -f "$H/.ssh/id_ed25519"
ssh-keygen -q -t ed25519 -N '' -f "$E2E_DIR/sshd/host_ed25519"
cp "$H/.ssh/id_ed25519.pub" "$E2E_DIR/sshd/authorized_keys"

cat >"$E2E_DIR/sshd/sshd_config" <<EOF
Port $SSH_PORT
ListenAddress 127.0.0.1
HostKey $E2E_DIR/sshd/host_ed25519
PidFile $E2E_DIR/sshd/sshd.pid
AuthorizedKeysFile $E2E_DIR/sshd/authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
StrictModes no
Subsystem sftp internal-sftp
EOF

# Absolute paths: ssh expands ~ from the passwd entry, not $HOME.
cat >"$H/.ssh/config" <<EOF
Host testbox
  HostName 127.0.0.1
  Port $SSH_PORT
  User $(id -un)
  IdentityFile $H/.ssh/id_ed25519
  UserKnownHostsFile $H/.ssh/known_hosts
EOF

echo "hello from portunus" >"$H/Documents/hello.txt"
head -c 3145728 /dev/urandom >"$H/Documents/random.bin"
for i in $(seq 1 12); do echo "photo $i" >"$H/Documents/photos/2026/img$i.jpg"; done
echo "existing remote" >"$E2E_DIR/remote/hello.txt"
echo "only on remote" >"$E2E_DIR/remote/remote-only.txt"

/usr/sbin/sshd -f "$E2E_DIR/sshd/sshd_config" -E "$E2E_DIR/sshd/log"

echo "== minio"
MINIO_ID="$(docker run -d --rm -p 127.0.0.1::9000 --tmpfs /data:rw,mode=1777 --entrypoint minio \
  -e MINIO_ROOT_USER=$MINIO_USER -e MINIO_ROOT_PASSWORD=$MINIO_PASS \
  "$MINIO_IMAGE" server /data --address :9000)"
MINIO_ADDR="$(docker port "$MINIO_ID" 9000/tcp | head -1)"
for _ in $(seq 1 50); do
  curl -fs "http://$MINIO_ADDR/minio/health/live" >/dev/null && break
  sleep 0.2
done

echo "== server build"
(cd "$ROOT" && go build -tags server -o "$E2E_DIR/portunus-server" .)
HOME="$H" XDG_CONFIG_HOME="$E2E_DIR/config" PORTUNUS_SSH_CONFIG="$H/.ssh/config" \
  WAILS_SERVER_PORT="$APP_PORT" "$E2E_DIR/portunus-server" >"$E2E_DIR/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 50); do
  curl -fs "http://127.0.0.1:$APP_PORT/" >/dev/null && break
  sleep 0.2
done

echo "== playwright"
rm -rf "$OUT" && mkdir -p "$OUT"
status=0
docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -v "$ROOT/e2e:/work" -v "$OUT:/out" -v "$E2E_DIR:$E2E_DIR" -w /work \
  -e APP_URL="http://127.0.0.1:$APP_PORT/" -e E2E_DIR="$E2E_DIR" \
  -e MINIO_ENDPOINT="http://$MINIO_ADDR" -e MINIO_USER=$MINIO_USER -e MINIO_PASS=$MINIO_PASS \
  "$PW_IMAGE" bash -c '[ -d node_modules/playwright ] || npm i --no-save --silent playwright@1.65.0; node flows/sftp.mjs && node flows/s3.mjs' || status=$?

if [[ $status -ne 0 ]]; then
  echo "== server log"; tail -50 "$E2E_DIR/server.log"
  exit $status
fi
echo "== e2e passed; screenshots in $OUT"
