package session

import "testing"

// Real panes captured from Claude Code v2.1.227 on a ClaudeBox droplet.
const (
	paneTheme = `
 Let's get started.
 Choose the text style that looks best with your terminal:
 To change this later, run /theme
 ❯ 1. Dark mode
   2. Light mode
`
	paneLoginMethod = `
Welcome to Claude Code v2.1.227
 Claude Code can be used with your Claude subscription or billed based on API
 usage through your Console account.
 Select login method:
 ❯ 1. Claude account with subscription · Pro, Max, Team, or Enterprise
   2. Anthropic Console account · API usage billing
`
	paneOAuthURL = `
Welcome to Claude Code v2.1.227
 Browser didn't open? Use the url below to sign in (c to copy)
https://claude.com/cai/oauth/authorize?code=true&client_id=9d1c250a
 Paste code here if prompted >
`
	paneTrustFolder = `
 Accessing workspace:
 /workspace/master
 Quick safety check: Is this a project you created or one you trust?
 Claude Code'll be able to read, edit, and execute files here.
 ❯ 1. Yes, I trust this folder
   2. No, exit
`
	paneBypassWarning = `
  WARNING: Claude Code running in Bypass Permissions mode
  In Bypass Permissions mode, Claude Code will not ask for your approval
  before running potentially dangerous commands.
  ❯ 1. No, exit
    2. Yes, I accept
  Enter to confirm · Esc to cancel
`
	paneFullscreen = `
  Try the new fullscreen renderer?
  · Flicker-free output — fixes the flashing you see during long responses
  · Mouse support — click to move your cursor or expand results
  ❯ 1. Yes, try it
    2. Not now
`
	paneReady = `
   +1 more · /status
❯ Try "fix typecheck errors"
────────────────────────────────────────────────────────────────────
  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents
`
	paneStillLoading = `
Welcome to Claude Code v2.1.227
 Loading...
`
)

func TestClassifyPane(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want paneAction
	}{
		{"theme picker is confirmed", paneTheme, actionEnter},
		{"trust prompt is confirmed", paneTrustFolder, actionEnter},
		{"bypass warning is accepted", paneBypassWarning, actionSelectSecond},
		{"fullscreen renderer is declined", paneFullscreen, actionSelectSecond},
		{"main prompt is ready", paneReady, actionReady},
		{"unrecognised screen keeps waiting", paneStillLoading, actionWait},
		// A logged-out claude user cannot be rescued from here: cbx code has no
		// way to complete an OAuth flow, so it must not blunder into one.
		{"login method means not authenticated", paneLoginMethod, actionNotAuthenticated},
		{"oauth url means not authenticated", paneOAuthURL, actionNotAuthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPane(tt.pane); got != tt.want {
				t.Errorf("classifyPane() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The bypass warning screen and the ready status line both mention bypass
// permissions; classifying the warning as ready would leave the session parked
// on a modal that never gets answered.
func TestClassifyPane_BypassWarningIsNotReady(t *testing.T) {
	if got := classifyPane(paneBypassWarning); got == actionReady {
		t.Error("bypass warning classified as ready — the modal is never answered")
	}
}

// The old implementation treated any pane containing ">" as ready, which
// matched the OAuth paste prompt and every other screen.
func TestClassifyPane_ChevronAloneIsNotReady(t *testing.T) {
	if got := classifyPane("some banner > with a chevron"); got == actionReady {
		t.Error(`a bare ">" was treated as ready`)
	}
}
