# Decision log

Choices made while building the two-binary redesign, recorded so they can be
reviewed or reversed. Newest last. Each entry says what was decided, why, and
what would make it wrong.

## SQLite driver: `modernc.org/sqlite`

Pure Go, so `CGO_ENABLED=0 GOOS=linux` cross-compiles from a Mac — which is how
release binaries are built. A cgo driver (`mattn/go-sqlite3`) would break that.

Validated before committing: built and ran a scratch program natively, then
cross-compiled it to linux/amd64. Costs ~7MB of binary. Acceptable for
something cloud-init downloads once.

**Wrong if** the binary size becomes a problem, or the box ever needs a driver
feature modernc lacks.

## SQLite pragmas go in the DSN, not an `Exec`

`PRAGMA busy_timeout` set via `db.Exec` applies only to whichever pooled
connection served it. `database/sql` opens others on demand and they get
defaults, so concurrent writers still fail instantly.

Found by test, not by reading: the concurrency test lost 19 of 20 writes to
`SQLITE_BUSY`. Moving the pragmas into the DSN fixed it, and reverting them in
a scratch worktree reproduced the failure — so the test discriminates.

## Session database lives under `XDG_STATE_HOME`

`~/.local/state/cbx/sessions.db`, not `~/.config`. It is generated data cbx can
rebuild by reconciling against tmux, which is state rather than configuration.

## `tmux.Client.Start` refuses to replace a running session

The old code killed any existing session of the same name first. That silently
destroys work in progress. Reattaching is `resume`'s job; if you really want it
gone, `kill` says so explicitly.

## The tmux command is injectable

`Client.WithCommand` exists so tests drive real tmux with something cheap
instead of Claude Code. This is what makes `cbx` fully testable on a laptop,
which was a stated requirement — the package has no remote dependency at all.

## `Kill` is idempotent, `Get` on a missing session is not an error

Both are things an agent asks about routinely. Returning an error for "already
gone" or "not found" makes callers write error-handling for the normal case.

## `--dangerously-skip-permissions` stays the default, but is named

An automated security review flagged it as HIGH. Keeping it: these sessions are
driven from a phone with nobody at a terminal, so a permission prompt has no
one to answer and the session just stalls. That is the product, and the box is
single-tenant and owned by whoever ran cbx.

Taking the reviewer's real point though — it was hidden inside a constructor.
It is now a named constant with the reasoning attached, and
`WithPermissionPrompts()` turns it off. A deliberate risk should be visible at
the point it is taken.

**Wrong if** ClaudeBox ever hosts sessions for someone other than the box's
owner, at which point the blast radius stops being self-inflicted.

## Integration and smoke tests are required, not optional

Standing instruction: every part of this must be validated by integration tests
locally and a smoke test against a real droplet before it counts as done. The
last large refactor passed 181 unit tests and a mutation-tested review, then
forty minutes on a real box found six defects — three blocking — because every
test used a fake and fakes cannot model a shell, a PATH, or an installer that
exits 0 having done nothing.

Droplets may be created freely for this. **Every droplet except `claudebox`
must be destroyed when the work is finished.**

## Commit hygiene: stage explicit paths

`git add -A` twice swept a subagent's or my own in-flight work into an
unrelated commit — once putting 209 lines of Go inside a commit labelled
`docs:`. Both were caught and rewritten before pushing. The rule now is
`git commit --only <paths>`, which commits the named paths regardless of what
else is staged.

## Removed `internal/shell`'s PATH-clobbering `init()`

Not a planned change — it blocked the work. A new integration test kept
skipping with "tmux not installed" on a machine where tmux is installed. The
cause: `shell.init()` ran `os.Setenv("PATH", FullPATH)`, so importing the
package anywhere in a binary replaced the PATH of the whole process with a
hardcoded Linux list. `/opt/homebrew/bin` vanished and with it tmux and gh. The
test package never imported `shell` directly; it arrived transitively.

The `init()` was redundant besides — `RunShell` already sets the PATH per
command. It only affected `exec.LookPath`, so `Which` now resolves against
`FullPATH` explicitly instead.

An AST test asserts the package declares no `init()`, because this is invisible
until something far away breaks.

**Note for the eventual split:** `FullPATH` still hardcodes `/root/...`, which
is wrong for any non-root user. `internal/tmux` takes the other approach —
prepending `$HOME/.local/bin` inside the script so the shell expands it. The
`FullPATH` constant should follow when the setup tool moves.
