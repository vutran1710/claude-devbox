package tmux

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// These tests drive the real tmux on this machine. cbx has no remote
// dependency, so it is fully testable on a laptop — that is the point of
// splitting it out of the setup tool.

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := Shell("command -v tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

// live returns a Client that starts a cheap long-running command instead of
// Claude Code, and cleans up whatever it created.
func live(t *testing.T) (*Client, string) {
	t.Helper()
	requireTmux(t)
	c := New().WithCommand("sleep 300")
	name := fmt.Sprintf("cbxtest-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
	t.Cleanup(func() { c.Kill(name) })
	return c, name
}

func TestStartThenHasThenKill(t *testing.T) {
	c, name := live(t)

	if c.Has(name) {
		t.Fatalf("%q already exists before the test started", name)
	}
	if err := c.Start(name, os.TempDir()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !c.Has(name) {
		t.Error("Has = false immediately after Start")
	}
	if err := c.Kill(name); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if c.Has(name) {
		t.Error("Has = true after Kill")
	}
}

// Killing a session that was never started is the normal case for an agent
// cleaning up, and must not be an error.
func TestKillIsIdempotent(t *testing.T) {
	c, name := live(t)
	if err := c.Kill(name); err != nil {
		t.Errorf("Kill on an absent session: %v", err)
	}
}

// The old code killed any existing session of the same name before starting,
// which silently destroys work in progress.
func TestStartRefusesToReplaceARunningSession(t *testing.T) {
	c, name := live(t)
	if err := c.Start(name, os.TempDir()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	err := c.Start(name, os.TempDir())
	if err == nil {
		t.Fatal("second Start succeeded — a running session was silently replaced")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error = %q, want it to say the session is already running", err)
	}
	if !c.Has(name) {
		t.Error("the original session died despite the refusal")
	}
}

func TestListIncludesAStartedSession(t *testing.T) {
	c, name := live(t)
	if err := c.Start(name, os.TempDir()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	names, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, n := range names {
		if n == name {
			found = true
		}
	}
	if !found {
		t.Errorf("List = %v, missing %q", names, name)
	}
}

func TestCaptureReadsThePane(t *testing.T) {
	c, name := live(t)
	c2 := c.WithCommand(`bash -c 'echo CBX_MARKER_9f3a; sleep 300'`)
	if err := c2.Start(name, os.TempDir()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// The command needs a moment to emit before the pane has anything.
	var pane string
	for i := 0; i < 20; i++ {
		pane, _ = c2.Capture(name, 20)
		if strings.Contains(pane, "CBX_MARKER_9f3a") {
			return
		}
		Shell("sleep 0.2")
	}
	t.Errorf("marker never appeared in the pane; got:\n%s", pane)
}

// A session name is user input that reaches tmux through a shell.
func TestNamesWithShellMetacharactersAreQuotedNotExecuted(t *testing.T) {
	requireTmux(t)
	canary := os.TempDir() + "/cbx-canary-should-not-exist"
	os.Remove(canary)

	c := New().WithCommand("sleep 300")
	name := "x; touch " + canary
	t.Cleanup(func() { c.Kill(name); os.Remove(canary) })

	// Whether tmux accepts this name does not matter; the shell must not run
	// the touch.
	c.Start(name, os.TempDir())
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("a session name executed as a shell command — quoting is broken")
	}
}

func TestFindRemoteControlURL(t *testing.T) {
	for _, tc := range []struct{ pane, want string }{
		{"noise\n  https://claude.ai/code/session_01ABC  \nmore", "https://claude.ai/code/session_01ABC"},
		{"https://claude.com/code/session_XYZ", "https://claude.com/code/session_XYZ"},
		{"nothing here", ""},
		{"a https://example.com/code/session_x b", ""},
	} {
		if got := FindRemoteControlURL(tc.pane); got != tc.want {
			t.Errorf("FindRemoteControlURL(%q) = %q, want %q", tc.pane, got, tc.want)
		}
	}
}
