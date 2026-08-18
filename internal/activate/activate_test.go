package activate

import (
	"errors"
	"strings"
	"testing"

	"github.com/vutran1710/claudebox/internal/master"
	"github.com/vutran1710/claudebox/internal/ui"
)

func newTestModel() activateModel {
	return activateModel{
		spinner: ui.NewSpinner(),
		steps:   []ui.Step{{Name: "Start the master session", State: ui.StepRunning}},
	}
}

func TestActivateReportsRemoteControlURL(t *testing.T) {
	m, _ := newTestModel().Update(claudeSessionReadyMsg{rcURL: "https://claude.ai/code/abc"})
	got := m.(activateModel)

	if !got.done {
		t.Error("model should be done once the session is up")
	}
	if got.steps[0].State != ui.StepDone {
		t.Errorf("step state = %v, want StepDone", got.steps[0].State)
	}
	view := got.View()
	if !strings.Contains(view, "https://claude.ai/code/abc") {
		t.Errorf("view does not show the Remote Control URL:\n%s", view)
	}
	// The URL is the whole point of the command — without the session name the
	// user cannot attach to it over SSH.
	if !strings.Contains(view, master.Name) {
		t.Errorf("view does not name the %s session:\n%s", master.Name, view)
	}
}

// A session that failed to start must not be reported as running — the user
// would attach to a session that is not there.
func TestActivateSurfacesStartupFailure(t *testing.T) {
	m, _ := newTestModel().Update(ui.ErrMsg{Err: errors.New("not logged in")})
	got := m.(activateModel)

	if got.done {
		t.Error("model marked done despite the session failing to start")
	}
	if got.steps[0].State != ui.StepError {
		t.Errorf("step state = %v, want StepError", got.steps[0].State)
	}
	if !strings.Contains(got.View(), "not logged in") {
		t.Errorf("view hides the failure reason:\n%s", got.View())
	}
}
