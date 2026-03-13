<p align="center">
  <img src="logo.svg" width="200" />
</p>

# Claude DevBox

A dedicated remote development server deployed on DigitalOcean with Claude Code CLI and essential dev tools pre-installed.

## Why

I stopped writing code. I stopped reading code. Claude does both now.

So why am I still carrying a Macbook around like I'm the one who needs the compute?

Claude DevBox puts the dev environment where it belongs — on a server, in the cloud, accessible from anywhere. SSH in from an iPad, a Chromebook, your phone, whatever. Claude writes the code, agent-browser tests it, wormhole shares it. Your job is to think and type directions.

The Macbook stays home. You don't.

## What's Included

| Tool | Purpose |
|------|---------|
| Claude Code CLI | AI coding assistant |
| Agent Browser | Headless browser automation for testing web apps |
| GitHub CLI | Git operations, PRs, issues |
| Vercel CLI | Frontend deployments |
| Supabase CLI | Backend/database management |
| Docker | Container management |
| Wormhole | Tunnel localhost to internet (HTTPS) |
| Rust + Cargo | Systems programming |
| Python 3.13 + uv | Python dev with fast package management |
| Node.js 22 | JavaScript runtime |
| Go | For Go-based tooling |

## Deploy to DigitalOcean

### Via GitHub Actions (recommended)

1. Push this repo to GitHub
2. Create a DigitalOcean API token from the [API settings](https://cloud.digitalocean.com/account/api/tokens)
3. Add these **GitHub repo secrets** (Settings → Secrets → Actions):
   - `DIGITALOCEAN_ACCESS_TOKEN` — your DigitalOcean API token
   - `SSH_PUBLIC_KEY` — your public key (`cat ~/.ssh/id_ed25519.pub`)
4. Go to Actions → "Deploy to DigitalOcean" → Run workflow
5. SSH in and attach to the tmux session:

```bash
ssh root@<droplet-ip>
tmux attach -t claude
# Complete the OAuth login, then you're ready
```

### Manual deploy

```bash
# Install doctl (DigitalOcean CLI)
# macOS: brew install doctl
# Linux: snap install doctl

# Authenticate
doctl auth init

# Create a Droplet with Docker pre-installed
doctl compute droplet create claude-devbox \
  --image docker-20-04 \
  --size s-4vcpu-8gb \
  --region nyc1 \
  --ssh-keys <your-ssh-key-fingerprint>

# SSH in and build
ssh root@<droplet-ip>
git clone https://github.com/your/claude-devbox.git /opt/claude-devbox
cd /opt/claude-devbox
docker build -t claude-devbox .
docker run -d --name claude-devbox --restart unless-stopped \
  -p 22:22 -e SSH_PUBLIC_KEY="$(cat ~/.ssh/id_ed25519.pub)" claude-devbox
```

## tmux Basics

```bash
tmux attach -t claude     # Attach to the claude session
# Ctrl+B then D           # Detach (keeps running)
tmux new -s work          # Create another session
tmux ls                   # List sessions
```

## Usage

SSH into the server, then:

```bash
# Start coding with Claude
cd /workspace
git clone https://github.com/your/project.git
cd project
claude

# Expose dev server to internet
wormhole http 3000

# Test the app with headless browser
agent-browser open "http://localhost:3000"
agent-browser screenshot /tmp/page.png
```

## Ports

Port 22 (SSH) is exposed by default. All other ports are accessible directly on the Droplet's IP — expose whatever you need through DigitalOcean's firewall settings or tunnel them via wormhole.
