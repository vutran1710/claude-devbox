# ClaudeBox TODO

## Phase 2: Refactor & Control Plane (feat/plugin-architecture branch)

**Full plan:** [docs/refactor-plan.md](refactor-plan.md)

### Refactor (Steps 1-6)
- [x] Step 1: Shell utilities (foundation) — 6 tests
- [x] Step 2: Extract `workspace/` — repo & project resolution — 5 tests
- [x] Step 3: Extract `session/` — clean Manager interface — 5 tests
- [x] Step 4: Extract `service/` — unified Service interface — 5 tests
- [x] Step 5: Clean `auth/` — OAuth + API keys only — 5 tests
- [x] Step 6: Create `provision/` — tools + user creation

### New Implementation (Steps 7-10)
- [x] Step 7: Rewrite `api/` — HTTP server with interfaces — 11 tests
- [x] Step 8: CLI uses new packages
- [x] Step 9: Wire everything in `cmd/cbx/main.go`
- [x] Step 10: Integration tests — 4 tests + E2E script

### Feature Requirements
- [x] `name` required in POST /sessions
- [x] Response includes `dir` and `status` (cloned/found/created/already running)
- [x] Cloudflare tunnel for cbx serve
- [x] Setup: one command, prints VNC URL + skill file
- [x] No master session — sessions created on-demand
- [x] Unified `cbx code <name> [--repo]` — smart resolution
- [x] Duplicate session check
- [x] gh auth via token file (cloud-init → setup)
- [x] Skill-formatted output ready to save as SKILL.md

## Done

### ClaudeBox core (main branch)
- [x] `cbx setup` — install tools, OAuth, VNC, master session
- [x] `cbx activate` — start the claude-main session
- [x] `cbx code` — spawn sessions with -g (GitHub) and -p (project)
- [x] `cbx code --headless` — non-interactive mode for master session
- [x] `cbx status` — show all services + sessions
- [x] `cbx show api-key` — view serve API key
- [x] `cbx serve` — HTTP API daemon with auth (POST/GET/DELETE sessions)
- [x] Cobra CLI, version injection from git tag
- [x] Cloud-init wait, dpkg lock handling, unattended-upgrades
- [x] Deploy/undeploy via GitHub Actions (DO + Railway, default Singapore)
- [x] Cloudflare tunnel for VNC

## Future

### Features
- [ ] Rate limiting on serve API
- [ ] Session auto-cleanup (kill idle sessions)
