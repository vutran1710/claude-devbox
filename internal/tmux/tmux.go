// Package tmux drives tmux so Claude does not have to.
//
// Everything here runs as whoever invoked cbx. There is no identity switching:
// the account that starts a session is the account that owns it, which is what
// keeps `tmux ls` honest for the person reading it.
package tmux

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// toolPath puts the directories installers write into ahead of the system
// PATH. A non-interactive shell reads no rc file, so ~/.local/bin — where the
// Claude Code installer puts claude — is otherwise invisible. $HOME is
// expanded by the shell, so this holds for whoever is running.
const toolPath = `export PATH="$HOME/.local/bin:$HOME/.npm-global/bin:$HOME/.cargo/bin:/usr/local/go/bin:$PATH"; `

// Runner executes a shell script and returns its combined output. Injected so
// tests can observe commands without a tmux server, though most of this
// package's tests drive the real one.
type Runner func(script string) (string, error)

// Shell runs a script through bash.
func Shell(script string) (string, error) {
	out, err := exec.Command("bash", "-c", toolPath+script).CombinedOutput()
	return string(out), err
}

type Client struct {
	run Runner
	// command is what a session runs. Overridable so tests can start something
	// cheap instead of Claude Code.
	command string
}

// autonomousClaude is what a session runs by default.
//
// --dangerously-skip-permissions is deliberate and is the product's premise:
// these sessions are driven from a phone with nobody at a terminal, so a
// permission prompt has no one to answer it and the session simply stalls.
// The box is single-tenant and owned by the person running cbx.
//
// It is named here rather than buried in a constructor so the choice is
// visible, and WithPermissionPrompts turns it off for anyone who wants the
// prompts back.
const autonomousClaude = "claude --dangerously-skip-permissions"

func New() *Client { return &Client{run: Shell, command: autonomousClaude} }

// WithPermissionPrompts runs Claude with its permission prompts intact. A
// session started this way will block until someone answers, so it is only
// useful when a human is attached to the tmux session.
func (c *Client) WithPermissionPrompts() *Client { c.command = "claude"; return c }

// WithRunner and WithCommand exist for tests.
func (c *Client) WithRunner(r Runner) *Client  { c.run = r; return c }
func (c *Client) WithCommand(s string) *Client { c.command = s; return c }

// quote wraps s for safe interpolation into a shell script. Session names and
// directories reach tmux through a shell, and a name is user input.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Has reports whether a session of that name is running.
func (c *Client) Has(name string) bool {
	_, err := c.run("tmux has-session -t " + quote(name) + " 2>/dev/null")
	return err == nil
}

// List returns the names of running sessions.
func (c *Client) List() ([]string, error) {
	out, err := c.run(`tmux list-sessions -F '#{session_name}' 2>/dev/null`)
	if err != nil {
		// No server running is an empty list, not a failure.
		return nil, nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// Start creates a detached session running the client's command in dir. It
// refuses to replace a running session — reattaching to work in progress is
// Resume's job, and silently killing it would lose whatever it was doing.
func (c *Client) Start(name, dir string) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}
	if c.Has(name) {
		return fmt.Errorf("session %q is already running — use resume to attach, or kill it first", name)
	}
	script := fmt.Sprintf("cd %s && tmux new-session -d -s %s %s",
		quote(dir), quote(name), quote(c.command))
	if out, err := c.run(script); err != nil {
		return fmt.Errorf("start session %q: %w: %s", name, err, strings.TrimSpace(out))
	}
	return nil
}

// Kill stops a session. Killing one that is not running is not an error: the
// caller's intent is that it be gone.
func (c *Client) Kill(name string) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}
	c.run("tmux kill-session -t " + quote(name) + " 2>/dev/null")
	if c.Has(name) {
		return fmt.Errorf("session %q is still running after kill", name)
	}
	return nil
}

// Capture returns the last n lines of a session's pane.
func (c *Client) Capture(name string, lines int) (string, error) {
	out, err := c.run(fmt.Sprintf("tmux capture-pane -t %s -p -J -S -%d", quote(name), lines))
	if err != nil {
		return "", fmt.Errorf("capture pane of %q: %w", name, err)
	}
	return out, nil
}

// SendKeys types into a session.
func (c *Client) SendKeys(name, keys string) error {
	_, err := c.run(fmt.Sprintf("tmux send-keys -t %s %s", quote(name), keys))
	return err
}

// LoggedIn reports whether Claude Code is signed in for the current user.
//
// Checked before waiting for a Remote Control URL: an unauthenticated session
// starts fine and then never produces one, and a bare 60-second timeout does
// not tell anybody why.
func (c *Client) LoggedIn() bool {
	out, err := c.run(`claude auth status --json 2>/dev/null`)
	if err != nil {
		return false
	}
	return strings.Contains(out, `"loggedIn": true`) || strings.Contains(out, `"loggedIn":true`)
}

var rcURL = regexp.MustCompile(`https://claude\.(?:com|ai)/code/\S+`)

// FindRemoteControlURL extracts a Remote Control URL from pane text. Split out
// from the driving logic so it can be tested against real captured panes.
func FindRemoteControlURL(pane string) string {
	return rcURL.FindString(pane)
}

// startupPrompts are the questions Claude Code asks before it reaches its
// input line. Each is answered by pressing Enter to take the highlighted
// default, which is the affirmative in every case.
//
// The trust prompt is not an edge case: cbx new creates the project directory,
// so Claude has never seen it before and asks every single time. Typing
// /remote-control into that prompt does nothing, which is how a session ends
// up running with no URL.
var startupPrompts = []string{
	"Is this a project you created or one you trust",
	"Choose the text style",
	"Security notes",
	"Enable Remote Control",
}

// EnableRemoteControl runs /remote-control in a session and returns the URL it
// prints.
//
// It answers startup prompts as it sees them rather than assuming the session
// is at its input line, and polls for the URL rather than reading the pane
// once — the URL takes a few seconds to arrive.
func (c *Client) EnableRemoteControl(name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	answered := map[string]bool{}
	var lastAsk time.Time

	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		pane, err := c.Capture(name, 60)
		if err != nil {
			continue
		}
		if url := FindRemoteControlURL(pane); url != "" {
			return url, nil
		}

		// Clear whatever question is on screen before typing anything else.
		if cleared := c.answerPrompt(name, pane, answered); cleared {
			continue
		}

		// Only ask once Claude is actually at its input line. Sending into a
		// session that is still booting silently does nothing, and asking once
		// and assuming it landed is how a session ends up with no URL.
		if !isReady(pane) {
			continue
		}
		if time.Since(lastAsk) < 15*time.Second {
			continue
		}

		// Text and Enter go separately with a pause. Sent together, Claude
		// receives the Enter before it has registered /remote-control as a
		// slash command and treats it as a prompt — it answers by searching
		// the filesystem for a command definition, and no URL appears.
		if err := c.SendKeys(name, quote("/remote-control")); err != nil {
			return "", err
		}
		time.Sleep(2 * time.Second)
		if err := c.SendKeys(name, "Enter"); err != nil {
			return "", err
		}
		lastAsk = time.Now()
	}
	return "", fmt.Errorf("no Remote Control URL appeared in %q within %s", name, timeout)
}

// answerPrompt presses Enter on the first unanswered startup question it finds,
// reporting whether it did.
func (c *Client) answerPrompt(name, pane string, answered map[string]bool) bool {
	for _, p := range startupPrompts {
		if !answered[p] && strings.Contains(pane, p) {
			c.SendKeys(name, "Enter")
			answered[p] = true
			return true
		}
	}
	return false
}

// isReady reports whether Claude has finished starting and is showing its
// input line. The prompt marker and the mode line both appear only once it is
// accepting input.
func isReady(pane string) bool {
	return strings.Contains(pane, "❯") || strings.Contains(pane, "bypass permissions on") ||
		strings.Contains(pane, "for shortcuts")
}
