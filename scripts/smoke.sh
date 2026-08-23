#!/usr/bin/env bash
# Smoke test: provision a real droplet and drive it end to end.
#
# Unit tests use fakes, and fakes cannot model a shell, a PATH, an installer
# that exits 0 having done nothing, or Claude asking to trust a folder. Every
# serious bug in this project so far was found by running against a real box.
#
#   scripts/smoke.sh <host>          test an existing box
#   scripts/smoke.sh --create        create a droplet, test it, destroy it
#
# Never touches a droplet it did not create unless you name one explicitly.
set -euo pipefail

DROPLET_NAME="cbx-smoke-$$"
CREATED=""
HOST="${1:-}"

cleanup() {
  if [ -n "$CREATED" ]; then
    echo "--- destroying $DROPLET_NAME"
    doctl compute droplet delete "$DROPLET_NAME" --force >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok()   { echo "  ok   $*"; }

if [ "$HOST" = "--create" ]; then
  key=$(doctl compute ssh-key list --format ID --no-header | head -1)
  [ -n "$key" ] || fail "no SSH key registered with DigitalOcean"
  echo "--- creating $DROPLET_NAME"
  doctl compute droplet create "$DROPLET_NAME" --image ubuntu-24-04-x64 --size s-2vcpu-4gb \
    --region sgp1 --ssh-keys "$key" --wait >/dev/null
  CREATED=1
  HOST=$(doctl compute droplet get "$DROPLET_NAME" --format PublicIPv4 --no-header)
  for _ in $(seq 1 30); do nc -z -w5 "$HOST" 22 2>/dev/null && break; sleep 10; done
fi
[ -n "$HOST" ] || fail "usage: smoke.sh <host> | --create"

echo "--- building"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/cbx-smoke-linux ./cmd/cbx-next
go build -o /tmp/cbxst-smoke ./cmd/cbx-setuptool

echo "--- provisioning $HOST"
/tmp/cbxst-smoke setup --host "$HOST" --binary /tmp/cbx-smoke-linux --skip-claude-login --skip-auth

echo "--- checking the box"
remote() { ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 "root@$HOST" "$@"; }

for bin in node npm gh vercel supabase claude tmux git cbx; do
  remote "command -v $bin >/dev/null" || fail "$bin is not on PATH after setup"
  ok "$bin installed"
done

# The step verifier exists because an installer once exited 0 having installed
# nothing and still printed a tick.
remote 'cbx --version >/dev/null' || fail "cbx does not run"
ok "cbx runs"

remote 'test -d ~/.claude/skills' || fail "skills were not migrated"
ok "skills migrated"

# A settings.json still full of the operator's home directory would break every
# hook on the box.
if remote 'test -f ~/.claude/settings.json' 2>/dev/null; then
  remote 'grep -q "/Users/" ~/.claude/settings.json' && fail "settings.json still contains local /Users/ paths"
  ok "settings.json has no local paths"
fi

echo "--- session lifecycle"
remote 'cbx kill smoke >/dev/null 2>&1' || true
out=$(remote 'cbx new smoke 2>&1') || fail "cbx new failed: $out"
echo "$out" | grep -q "^name	smoke" || fail "cbx new did not report the session: $out"
ok "cbx new"

remote 'cbx ls' | grep -q '^smoke	running' || fail "cbx ls does not show the session running"
ok "cbx ls shows it running"

remote 'cbx resume smoke' | grep -q 'tmux attach -t smoke' || fail "cbx resume did not print the attach command"
ok "cbx resume"

remote 'cbx export db' | grep -q '^smoke' || fail "cbx export db does not include the session"
ok "cbx export db"

remote 'cbx kill smoke' || fail "cbx kill failed"
remote 'cbx ls' | grep -q '^smoke' && fail "session still listed after kill"
ok "cbx kill"

# Killing something already gone is the normal case for an agent cleaning up.
remote 'cbx kill smoke' || fail "kill is not idempotent"
ok "kill is idempotent"

echo
echo "PASS — $HOST"
