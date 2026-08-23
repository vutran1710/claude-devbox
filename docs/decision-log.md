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
