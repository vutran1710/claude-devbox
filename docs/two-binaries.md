# Two binaries: `cbx-setuptool` and `cbx`

## Why

`cbx` is currently one binary doing two unrelated jobs, and every awkward thing
about it traces back to that.

It provisions a machine — installs tools, authenticates Claude, starts VNC —
which is a thing a person does occasionally, from their laptop, watching it
happen. And it manages tmux sessions, which is a thing an agent does
constantly, on the box, without a human present.

Those have opposite requirements. Provisioning wants a progress display, a
place to paste an auth code, and a confirmation before it touches a remote
machine. Session management wants none of that: an agent needs an exit code and
a line it can parse, and must never be handed a prompt it cannot answer.

Symptoms of the collision, all observed:

- `cbx setup --ip` had to be bolted on so a laptop could drive a remote box,
  because the binary assumed it was already on the box.
- `cbx code --headless` exists because the same command needs a TUI for a human
  and no TUI for the master session. The master-session retry message once
  omitted the flag, so the documented recovery step launched a TUI at an agent.
- A `shell.Runner` abstraction was built to let one binary act either locally or
  over SSH, and discarded as too large for what it bought.
- `cbx serve` published an HTTP API so something could drive sessions remotely,
  duplicating a capability the master session already has via its own shell.

Split the binary and each of these stops being a problem rather than being
solved.

## The split

```
laptop                                   the box
──────                                   ───────
cbx-setuptool ────────ssh───────▶        cbx new <name> [--repo <url>]
  interactive TUI                        cbx ls
  drives a remote machine                cbx kill <name>
  run occasionally, by a person          cbx resume <name>
                                           ▲
                                           │ called by the master Claude session
```

### `cbx-setuptool` — interactive, runs on your laptop

A Bubble Tea TUI. It never runs on the box; it drives one over SSH.

Responsibilities: create the droplet, install the tool chain, authenticate
Claude Code, push local `~/.claude` config, start VNC. It shows progress, it
asks before doing something to a remote machine, and it is where the auth code
gets pasted.

It does **not** manage sessions. To start the master session it runs
`cbx new master` on the box over SSH and relays the output — the session logic
lives in one place, on the machine the sessions run on.

### `cbx` — non-interactive, runs on the box

A plain CLI. No TUI, no spinners, no prompts, no confirmation steps. Its caller
is the master Claude session, which has a shell but no terminal and no human.

Responsibilities: wrap tmux so Claude never has to. Create a session, list
them, kill one, resume one. That is the whole surface.

Contract, because an agent depends on it:

- Never reads stdin. A missing argument is an error, never a prompt.
- Exit code is the result. Zero means it worked.
- Output is stable, one fact per line, parseable without a TUI.
- Errors name what to do next, on stderr.

It knows nothing about SSH, aliases, or remote machines. Anything remote is
`cbx-setuptool`'s job.

## `cbx` owns state on the box

tmux is the only session registry today, which means everything cbx knows about
a session dies when the tmux server does. That is not hypothetical: adding the
master session to the `docker` group required a `usermod` plus a full tmux
server restart, and the Remote Control URL changed because nothing had recorded
it.

`cbx` keeps a SQLite database on the box — one row per session: name, working
directory, repo it was cloned from, Remote Control URL, created-at, last-seen.
`cbx ls` reads it and reconciles against `tmux ls`, so it can distinguish a
session that is running from one that existed and died.

**Why SQLite rather than a JSON file.** Concurrent writes. The master session
and a human over SSH can both run `cbx` at once, and a read-modify-write over
a JSON file is a lost update — we already shipped exactly that bug in an
earlier attempt at a target store, then had to add file locking and atomic
renames by hand and got the cleanup path wrong twice. SQLite does this
correctly out of the box, and it is the difference between a store that is
right and one that is right until two commands overlap.

The cost is a dependency: the project currently has three, all Bubble Tea. A
pure-Go driver avoids cgo but is not small. See the open questions.

## Export: `cbx` reads out, `cbx-setuptool` writes in

The two binaries move config in opposite directions, and the asymmetry is
deliberate. `cbx-setuptool` pushes your laptop's `~/.claude` **to** a box.
`cbx` exports what is **on** the box, from the box, without needing a laptop:

```
cbx export skills     # skills installed here
cbx export rules      # CLAUDE.md and settings that shape sessions
cbx export db         # the session database
```

This is what makes a box inspectable and reproducible from the master session
itself — Claude can answer "what skills do I have?" and "what have I been
working on?" without anyone SSHing in. It is also the backup path: exporting
the db and the rules is enough to rebuild a box's identity somewhere else.

Output goes to stdout so it composes (`cbx export db > backup.sql`), which is
the same non-interactive contract as everything else in `cbx`.

## Where the fiddly parts live

Creating a session means driving `/remote-control` and reading the URL back out
of the pane. That involves a confirmation prompt and a retry loop, and it is the
most fragile code in the project. It lives in `cbx`, once. `cbx-setuptool`
bootstraps master by invoking `cbx new master` rather than reimplementing it.

## What this deletes

- `cbx serve`, the HTTP API, its Cloudflare tunnel, its API key, and
  `cbx show api-key`. The master session plus SSH are the control paths.
- `cbx activate`. It spawned a session, which `cbx new` does.
- `--headless`. `cbx` has no other mode.
- `cbx setup --ip`. `cbx-setuptool` always targets a remote.

## Open questions

1. **Is SQLite worth the dependency?** It solves concurrent writes correctly,
   which we demonstrably get wrong by hand. But `modernc.org/sqlite` is a large
   pure-Go dependency for a project with three, and cgo `mattn/go-sqlite3`
   complicates cross-compiling the linux binary from a Mac — which is how
   releases are built today.
2. **What exactly are "rules"?** `CLAUDE.md` alone, or also `settings.json`
   hooks, agents, and `.claude/` directories inside each workspace project?
   Export is only useful if it captures everything that shapes a session.
3. **Does `cbx export` have an `import` counterpart?** If the db and rules can
   be exported for backup, something has to restore them, and that is either
   `cbx import` or a `cbx-setuptool` job.
4. **How does `cbx-setuptool` reach the laptop?** `go install`, a release
   binary, or Homebrew. `cbx` is installed on the box by cloud-init as now.
5. **Does `cbx` need `--json`?** One-fact-per-line may be enough for an agent;
   JSON is easy to add and hard to remove.
6. **Does `resume` mean reattach, or restart a dead session?** They are
   different commands if a session can die.
7. **Does VNC survive?** It is a setuptool concern either way, but three
   browser stacks are installed today (Agent Browser, Playwright, VNC's
   Chromium) and at most one is load-bearing.
