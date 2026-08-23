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
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/cbx-smoke-linux ./cmd/cbx
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

# --repo goes through exec.Command with "--" rather than a shell, so a value
# git would read as an option must be refused and leave nothing behind.
remote 'rm -f /tmp/pwned; cbx new evil --repo "--upload-pack=touch /tmp/pwned"' 2>/dev/null && fail "a hostile --repo was accepted"
remote 'test -e /tmp/pwned' && fail "a hostile --repo executed its payload"
remote 'test -d "$HOME/workspace/evil"' && fail "a rejected clone left a directory behind"
ok "hostile --repo rejected cleanly"

# kill removes the session, not the directory — deleting someone's work is not
# kill's job — so the clone target has to be cleared explicitly here.
remote 'cbx kill demo >/dev/null 2>&1; rm -rf "$HOME/workspace/demo"'
remote 'cbx new demo --repo octocat/Hello-World >/dev/null' || fail "cbx new --repo failed"
remote 'test -f "$HOME/workspace/demo/README"' || fail "the repo was not cloned"
ok "cbx new --repo clones"
remote 'cbx kill demo >/dev/null 2>&1; rm -rf "$HOME/workspace/demo"' || true

remote 'cbx export skills' | grep -q . || fail "cbx export skills printed nothing"
ok "cbx export skills"
remote 'cbx export rules' | grep -q '=====' || fail "cbx export rules printed no sections"
ok "cbx export rules"

# The session store is SQLite rather than a JSON file precisely so overlapping
# callers do not lose each other's writes. Twelve at once, from separate
# processes.
remote 'for i in $(seq 1 12); do (cbx new conc$i >/dev/null 2>&1 &); done; sleep 25' || true
# Count only the concurrent ones: other sessions from earlier checks are still
# recorded, and an absolute total would make this assert the wrong thing.
n=$(remote 'cbx ls | grep -c "^conc"' | tr -d ' \r')
[ "$n" = "12" ] || fail "concurrent cbx new recorded $n of 12 sessions — writes were lost"
ok "12 concurrent sessions, no lost writes"
remote 'for i in $(seq 1 12); do cbx kill conc$i >/dev/null 2>&1; done' || true

remote 'cbx new smoke >/dev/null 2>&1' || true
remote 'cbx kill smoke' || fail "cbx kill failed"
remote 'cbx ls' | grep -q '^smoke' && fail "session still listed after kill"
ok "cbx kill"

# Killing something already gone is the normal case for an agent cleaning up.
remote 'cbx kill smoke' || fail "kill is not idempotent"
ok "kill is idempotent"

echo
echo "PASS — $HOST"
