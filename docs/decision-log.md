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

## Three Remote-Control bugs, all found by running the binary

None of these were caught by tests. Each produced a session that started fine
and then sat for a 60-second timeout with no URL. Found by running `cbx new` on
a Mac and reading the tmux pane.

1. **Claude asks to trust the folder.** `cbx new` creates the project
   directory, so Claude has never seen it and asks every single time.
   `/remote-control` typed into that prompt does nothing.
2. **Text and Enter cannot go in one `send-keys`.** Claude receives the Enter
   before it has registered `/remote-control` as a slash command, treats the
   whole thing as a prompt, and helpfully searches the filesystem for a command
   definition. They now go separately with a pause.
3. **Asking once is not enough.** The first version captured the pane
   immediately, saw a session still booting, found no prompt to answer, fired
   the command into nothing, and marked itself done. It now waits for Claude's
   input line to appear and re-asks every 15 seconds.

Bug 3 was briefly hidden by my own debug harness, which slept before its first
capture and so never reproduced the race. Worth remembering: an instrument that
does not match the code path can exonerate a bug.

All three now have regression tests, including one asserting the command and
the Enter are separate send-keys.

## `cbx` output format: tab-separated, one fact per line

`name\tvalue` for single results, `name\tstatus\tdir\turl` for lists. Chosen
over JSON because the caller is an agent with a shell: `cut -f2` and
`awk -F'\t'` work without a parser, and the format is readable when a person
looks at it. JSON is easy to add later as a flag and hard to remove.

## Shell-quoting is not argv-safety

`git clone` was built as a shell string with both arguments `shellQuote`d. That
is shell-safe and still wrong: quoting makes a value *one argv element*, and
that element can still be an option. `git clone --upload-pack=<cmd>` executes
`<cmd>`, so `cbx new x --repo '--upload-pack=touch /tmp/pwned'` was remote code
execution with perfectly correct quoting.

This is the third time the same distinction has bitten in this project — the
first was a host beginning with `-` reaching `ssh` as `-oProxyCommand=`, the
second a path reaching `scp`. Quoting defends the shell; it does nothing about
the program's own option parser.

The clone now goes through `exec.Command("git", "clone", "--", url, dir)` with
no shell at all, plus a validator that rejects a leading dash and requires
shorthand to look like `owner/repo`. Tests cover both the parser and the
end-to-end case, including that a rejected clone leaves no directory behind.

**Rule for the rest of this work:** if a value reaches a program that parses
options, either pass it after `--` via `exec.Command`, or reject a leading
dash. Quoting alone is not an answer.

## Tool tokens arrive on stdin, never in argv

`gh`, `vercel` and `supabase` are authenticated by piping a token over SSH into
the tool's login. A token passed as a command-line argument is visible in the
box's process table to every other user for as long as the command runs, and
lands in shell history.

The three tools disagree about how to accept one, which is exactly why the
recipes live in one table rather than scattered through provisioning:

- `gh auth login --with-token` reads stdin. Straightforward.
- `supabase login --token "$(cat)"` — the substitution happens in the remote
  shell, so the token is still never in cbx's argv.
- `vercel` has no stdin login at all; it reads `VERCEL_TOKEN` or `--token`.
  Its config file is written directly instead, with `umask 077` so it is
  private from creation rather than chmod-ed after a brief world-readable
  window. `printf` is a shell builtin, so even there the token never becomes
  the argv of a forked process.

Every recipe is followed by a verify command. Without one a bad or expired
token looks exactly like success, and the failure surfaces later inside a
session with no obvious cause.

Claude Code is deliberately absent from this list: subscription login is an
interactive browser OAuth with no token path. A test asserts it never appears
there, so nobody later "adds it for consistency".

Tokens are also read from the local environment (`GH_TOKEN`, `VERCEL_TOKEN`,
`SUPABASE_ACCESS_TOKEN`) so someone who already has them exported is not asked
to paste anything.

## setuptool prints progress rather than rendering a full-screen TUI

The brief said interactive, and it is — it prompts, pastes, and shows progress.
But it is not a Bubble Tea program that owns the screen, because the login step
hands the terminal to a nested Claude Code. A full-screen program has to be
suspended and restored around that, which is a lot of machinery for a flow a
person watches once per box. Line-by-line progress with `✓ · ✗` gives the same
information and composes with the interactive step instead of fighting it.

**Wrong if** setup grows steps that run concurrently or need live updating, at
which point a real TUI starts earning its complexity.

## Every install step is re-checked after it runs

`Step.Run` runs `Check` again after `Do`, whatever `Do`'s exit code was. A
`curl | bash` that exits 0 having installed nothing printed "✓ Supabase CLI" on
a real droplet where the binary existed nowhere on the filesystem.

The reverse is also handled: a `Do` that errors but whose `Check` then passes
is a success, because installers routinely exit non-zero on a harmless warning.

The supabase step was rewritten as a result — its official install script drops
a binary in the working directory rather than onto PATH, which is why it
appeared to succeed and left nothing behind.

## `InstallCBX` checks the ELF magic before uploading

Uploading a darwin build to a linux box fails later with "cannot execute binary
file", at a point far from the cause. Four bytes of magic turn that into an
error that names the fix.
