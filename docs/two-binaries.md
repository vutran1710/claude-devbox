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

1. **How does `cbx-setuptool` reach the laptop?** `go install`, a release
   binary, or Homebrew. `cbx` is installed on the box by cloud-init as now.
2. **Does `cbx` need `--json`?** One-fact-per-line may be enough for an agent;
   JSON is easy to add and hard to remove.
3. **Does `resume` mean reattach, or restart a dead session?** They are
   different commands if a session can die.
4. **Does VNC survive?** It is a setuptool concern either way, but three
   browser stacks are installed today (Agent Browser, Playwright, VNC's
   Chromium) and at most one is load-bearing.
