package session

import (
	"fmt"
	"strings"
	"time"
)

// navigateStartup answers Claude Code's startup screens until the session
// reaches its prompt. It fails rather than handing back a session parked on a
// screen it did not recognise — a session sitting on a login prompt looks
// running but cannot do any work.
func navigateStartup(name string) error {
	for i := 0; i < 20; i++ {
		switch classifyPane(capturPane(name)) {
		case actionReady:
			return nil
		case actionNotAuthenticated:
			return fmt.Errorf("the %s user is not logged in to Claude Code — run 'cbx setup'", ClaudeUser)
		case actionEnter:
			sendKeys(name, "Enter")
		case actionSelectSecond:
			sendKeys(name, "Down")
			time.Sleep(500 * time.Millisecond)
			sendKeys(name, "Enter")
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("session %q did not reach its prompt", name)
}

// paneAction is how to advance Claude Code's startup screens.
type paneAction int

const (
	actionWait             paneAction = iota // nothing recognised yet
	actionEnter                              // confirm the highlighted choice
	actionSelectSecond                       // move down one, then confirm
	actionReady                              // session is at its main prompt
	actionNotAuthenticated                   // claude user is logged out
)

// classifyPane decides how to advance a starting session.
//
// Order matters. The bypass-permissions warning and the ready status line both
// mention bypass permissions, so the warning is matched first; and the ready
// check keys off the status line rather than a bare ">", which used to match
// the OAuth paste prompt and hand back a session sitting on a login screen.
func classifyPane(pane string) paneAction {
	switch {
	// Logged out. cbx code cannot complete an OAuth flow, so report it
	// instead of typing into the login screen and starting one.
	case strings.Contains(pane, "Select login method"),
		strings.Contains(pane, "oauth/authorize"),
		strings.Contains(pane, "Paste code here"):
		return actionNotAuthenticated

	case strings.Contains(pane, "Choose the text style"):
		return actionEnter

	case strings.Contains(pane, "trust this folder"):
		return actionEnter

	// "1. No, exit" is highlighted by default — accept is the second option.
	case strings.Contains(pane, "Bypass Permissions mode"):
		return actionSelectSecond

	// Decline: the fullscreen renderer redraws in ways that break pane capture.
	case strings.Contains(pane, "fullscreen renderer"):
		return actionSelectSecond

	case strings.Contains(pane, "bypass permissions on"),
		strings.Contains(pane, "What can I help"):
		return actionReady
	}
	return actionWait
}
