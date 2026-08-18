package code

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/vutran1710/claudebox/internal/master"
	"github.com/vutran1710/claudebox/internal/session"
	"github.com/vutran1710/claudebox/internal/ui"
	"github.com/vutran1710/claudebox/internal/workspace"
)

// masterRequest reports whether this invocation targets the master session, and
// rejects the flags that do not apply to it. cbx code otherwise goes straight to
// TmuxManager.Create, which opens by killing the session it is asked for — so
// without this, cbx code master would destroy the session cbx setup and cbx
// activate deliberately leave alone.
func masterRequest(name, repo string) (bool, error) {
	if name != master.Name {
		return false, nil
	}
	if repo != "" {
		return true, fmt.Errorf("the %s session's workspace is managed by cbx — clone into a session of its own with 'cbx code <name> --repo %s'", master.Name, repo)
	}
	return true, nil
}

// runMaster hands the master session to the package that owns it, so every path
// that starts it makes the same decision about a session already running.
func runMaster() error {
	sess, err := master.Start()
	if err != nil {
		return err
	}
	if sess.Status == "already running" {
		fmt.Printf("Session '%s' already running\n", sess.Name)
	} else {
		fmt.Printf("Session '%s' started\n", sess.Name)
		fmt.Printf("Working dir: %s\n", sess.Dir)
	}
	if sess.RCURL != "" {
		fmt.Printf("Remote Control: %s\n", sess.RCURL)
	}
	fmt.Printf("Attach: tmux attach -t %s\n", sess.Name)
	return nil
}

// RunHeadless runs without TUI.
func RunHeadless(name, repo string) error {
	if isMaster, err := masterRequest(name, repo); err != nil {
		return err
	} else if isMaster {
		return runMaster()
	}

	result, err := workspace.Resolve(name, repo)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", result.Status, result.Dir)

	fmt.Printf("Starting session '%s'...\n", name)
	sess, err := session.NewTmuxManager().Create(name, result.Dir)
	if err != nil {
		return err
	}

	fmt.Printf("Session '%s' started\n", name)
	fmt.Printf("Working dir: %s\n", sess.Dir)
	if sess.RCURL != "" {
		fmt.Printf("Remote Control: %s\n", sess.RCURL)
	}
	fmt.Printf("Attach: tmux attach -t %s\n", name)
	return nil
}

// Run starts the TUI or headless mode.
func Run(name, repo string, headless bool) error {
	if isMaster, err := masterRequest(name, repo); err != nil {
		return err
	} else if isMaster {
		return runMaster()
	}
	if headless {
		return RunHeadless(name, repo)
	}

	m := model{
		spinner: ui.NewSpinner(),
		name:    name,
		repo:    repo,
	}
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// TUI model

type model struct {
	spinner spinner.Model
	name    string
	repo    string
	workDir string
	rcURL   string
	status  string
	done    bool
	err     error
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, resolveDir(m.name, m.repo))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case dirResolvedMsg:
		m.workDir = msg.dir
		m.status = msg.status
		return m, startSession(m.name, m.workDir)
	case sessionReadyMsg:
		m.rcURL = msg.rcURL
		m.done = true
		return m, tea.Quit
	case ui.ErrMsg:
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	if !m.done {
		if m.status != "" {
			fmt.Fprintf(&b, "  %s %s\n", ui.StyleCheck.Render(), m.status)
		}
		fmt.Fprintf(&b, "  %s Starting session '%s'...\n",
			ui.StyleSpin.Render(m.spinner.View()), m.name)
	} else {
		if m.status != "" {
			fmt.Fprintf(&b, "  %s %s\n", ui.StyleCheck.Render(), m.status)
		}
		fmt.Fprintf(&b, "  %s Session '%s' started\n", ui.StyleCheck.Render(), m.name)
		fmt.Fprintf(&b, "  %s Working dir: %s\n", ui.StyleCheck.Render(), m.workDir)
		if m.rcURL != "" {
			fmt.Fprintf(&b, "\n  Remote Control: %s\n", ui.StyleValue.Render(m.rcURL))
		}
		fmt.Fprintf(&b, "\n  Attach: tmux attach -t %s\n", m.name)
	}
	if m.err != nil {
		fmt.Fprintf(&b, "\n  %s %s\n", ui.StyleCross.Render(), m.err.Error())
	}
	return b.String() + "\n"
}

// Tea messages

type dirResolvedMsg struct {
	dir    string
	status string
}
type sessionReadyMsg struct{ rcURL string }

func resolveDir(name, repo string) tea.Cmd {
	return func() tea.Msg {
		result, err := workspace.Resolve(name, repo)
		if err != nil {
			return ui.ErrMsg{Err: err}
		}
		return dirResolvedMsg{dir: result.Dir, status: fmt.Sprintf("%s: %s", result.Status, result.Dir)}
	}
}

func startSession(name, workDir string) tea.Cmd {
	return func() tea.Msg {
		sess, err := session.NewTmuxManager().Create(name, workDir)
		if err != nil {
			return ui.ErrMsg{Err: err}
		}
		return sessionReadyMsg{rcURL: sess.RCURL}
	}
}
