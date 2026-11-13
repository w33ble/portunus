#!/usr/bin/env bash
# End-to-end smoke test: real sshd + RustFS (S3) + Wails server-mode build + Playwright (in Docker).
# Linux only (uses --network host). Screenshots land in e2e/out.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PW_IMAGE="${PW_IMAGE:-mcr.microsoft.com/playwright:v1.65.0-noble}"
S3_IMAGE="${PORTUNUS_S3_IMAGE:-rustfs/rustfs:1.0.5}"
OUT="$ROOT/e2e/out"
E2E_DIR="$(mktemp -d /tmp/portunus-e2e.XXXXXX)"
S3_USER=e2eadmin
S3_PASS=e2esecret123

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
  [[ -n "${S3_ID:-}" ]] && docker rm -f "$S3_ID" >/dev/null 2>&1 || true
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
# A passphrase-protected key makes the app ask for the passphrase.
ssh-keygen -q -t ed25519 -N e2epass -f "$H/.ssh/id_locked"
cat "$H/.ssh/id_ed25519.pub" "$H/.ssh/id_locked.pub" >"$E2E_DIR/sshd/authorized_keys"

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

# Own known_hosts so the host key prompt is still new after the sftp flow trusted testbox.
Host locked
  HostName 127.0.0.1
  Port $SSH_PORT
  User $(id -un)
  IdentityFile $H/.ssh/id_locked
  IdentitiesOnly yes
  UserKnownHostsFile $H/.ssh/known_hosts_locked
EOF

echo "hello from portunus" >"$H/Documents/hello.txt"
head -c 3145728 /dev/urandom >"$H/Documents/random.bin"
for i in $(seq 1 12); do echo "photo $i" >"$H/Documents/photos/2026/img$i.jpg"; done
echo "existing remote" >"$E2E_DIR/remote/hello.txt"
echo "only on remote" >"$E2E_DIR/remote/remote-only.txt"

/usr/sbin/sshd -f "$E2E_DIR/sshd/sshd_config" -E "$E2E_DIR/sshd/log"

# Buckets are created through the app (New folder at the S3 root), so no rc/mc setup is needed.
echo "== rustfs"
S3_ID="$(docker run -d --rm -p 127.0.0.1::9000 --tmpfs /data:rw,mode=1777 \
  -e RUSTFS_ACCESS_KEY=$S3_USER -e RUSTFS_SECRET_KEY=$S3_PASS \
  "$S3_IMAGE" /data)"
S3_ADDR="$(docker port "$S3_ID" 9000/tcp | head -1)"
for _ in $(seq 1 50); do
  curl -fs "http://$S3_ADDR/health" >/dev/null && break
  sleep 0.2
done

echo "== server build"
# Server mode is pure Go; with cgo on (any box with a C compiler) Wails pulls in GTK/WebKit code.
(cd "$ROOT" && CGO_ENABLED=0 go build -tags server -o "$E2E_DIR/portunus-server" .)
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
  -e E2E_FLOWS="${E2E_FLOWS:-sftp contrast s3 dialogs}" -e APP_URL="http://127.0.0.1:$APP_PORT/" -e E2E_DIR="$E2E_DIR" \
  -e S3_ENDPOINT="http://$S3_ADDR" -e S3_USER=$S3_USER -e S3_PASS=$S3_PASS \
  "$PW_IMAGE" bash -c 'for f in $E2E_FLOWS; do node flows/$f.mjs || exit 1; done' || status=$?

if [[ $status -ne 0 ]]; then
  echo "== server log"; tail -50 "$E2E_DIR/server.log"
  exit $status
fi
echo "== e2e passed; screenshots in $OUT"
