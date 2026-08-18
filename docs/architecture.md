# ClaudeBox Architecture

## Overview

ClaudeBox is a remote Claude Code server accessible from any device. It runs 24/7 in the cloud, manages Claude Code sessions, and optionally polls messaging apps.

## Components

```
Phone/Tablet/Laptop
  |
  ├── Claude App → Remote Control URL → Claude Code session
  ├── HTTP API  → cbx serve (port 8091) → session management
  └── SSH       → direct access
        |
        ▼
ClaudeBox (cloud server)
  |
  ├── cbx serve (daemon, port 8091)          ← control plane
  │   ├── POST /sessions                     ← create Claude Code session
  │   ├── GET  /sessions                     ← list sessions
  │   ├── DELETE /sessions/{name}            ← kill session
  │   └── Cloudflare tunnel → public URL
  │
  ├── Claude Code sessions (tmux, as the claude user)
  │   ├── master                             ← always-on session, Remote Control
  │   └── {project}                          ← project sessions
  │
  └── VNC desktop (port 6080)                ← visual access
      ├── Chrome
      └── Cloudflare tunnel → public URL
```

## Setup Flow

```
1. Deploy via GitHub Actions (DigitalOcean)
2. ssh -t root@host 'cbx setup'
   ├── Wait for cloud-init
   ├── Install all tools (15+)
   ├── Create claude user
   ├── OAuth authenticate
   ├── Start VNC + Chrome
   ├── Start cbx serve daemon + Cloudflare tunnel
   ├── Start the master session (Remote Control enabled)
   └── Print: Remote Control URL, VNC URL, API URL + key
3. Done — access from phone via Remote Control URL
```

## Session Management

All sessions go through `cbx serve`:

```
# Via API (from phone, another service, or the master session)
POST http://cbx-serve-url/sessions
  { "name": "my-project", "repo": "owner/repo" }
  → { "name": "my-project", "dir": "/workspace/my-project", "status": "created" }

# Via CLI (from SSH)
cbx code my-project --repo owner/repo
cbx code existing-project
```

## Ports

| Port | Service | Access |
|------|---------|--------|
| 22   | SSH | Direct (key auth) |
| 5900 | VNC | localhost → Cloudflare tunnel |
| 6080 | noVNC | localhost → Cloudflare tunnel |
| 8091 | cbx serve | localhost → Cloudflare tunnel |

## Repos

| Repo | Purpose |
|------|---------|
| [claudebox](https://github.com/vutran1710/claudebox) | Server setup, CLI, session management |
