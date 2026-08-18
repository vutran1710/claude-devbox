package activate

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/vutran1710/claudebox/internal/master"
	"github.com/vutran1710/claudebox/internal/ui"
)

type activateModel struct {
	spinner spinner.Model
	steps   []ui.Step
	rcURL   string
	status  string
	done    bool
	err     error
}

func Run() error {
	m := activateModel{
		spinner: ui.NewSpinner(),
		steps:   []ui.Step{{Name: "Start the master session", State: ui.StepRunning}},
	}
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func (m activateModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, doStartClaudeSession())
}

func (m activateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case claudeSessionReadyMsg:
		m.steps[0].State = ui.StepDone
		m.rcURL = msg.rcURL
		m.status = msg.status
		if msg.status == "already running" {
			m.steps[0].Name = "Master session already running"
		}
		m.done = true
		return m, tea.Quit
	case ui.ErrMsg:
		m.steps[0].State = ui.StepError
		m.steps[0].Error = msg.Err.Error()
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
}

func (m activateModel) View() string {
	var b strings.Builder
	b.WriteString(ui.StyleBold.Render("  ClaudeBox Activate") + "\n\n")
	b.WriteString(ui.RenderStepList(m.steps, m.spinner))
	if m.done {
		if m.rcURL != "" {
			b.WriteString("\n" + ui.RenderSummaryBox("Claude Code", []ui.KV{
				{Key: "Remote Control", Value: m.rcURL},
				{Key: "Session", Value: master.Name},
				{Key: "Status", Value: m.status},
			}))
		}
		b.WriteString("\n  " + ui.StyleBold.Render("Attach to session:") + "\n")
		fmt.Fprintf(&b, "    tmux attach -t %s\n", master.Name)
	}
	if m.err != nil {
		fmt.Fprintf(&b, "\n  %s %s\n", ui.StyleCross.Render(), m.err.Error())
	}
	return b.String() + "\n"
}

type claudeSessionReadyMsg struct{ rcURL, status string }

func doStartClaudeSession() tea.Cmd {
	return func() tea.Msg {
		sess, err := master.Start()
		if err != nil {
			return ui.ErrMsg{Err: err}
		}
		return claudeSessionReadyMsg{rcURL: sess.RCURL, status: sess.Status}
	}
}
