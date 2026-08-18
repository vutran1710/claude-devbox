package master

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/vutran1710/claudebox/internal/session"
)

type fakeManager struct {
	name    string
	workDir string
	creates int
	running bool
	err     error
}

func (f *fakeManager) Create(name, workDir string) (*session.Session, error) {
	f.name, f.workDir = name, workDir
	f.creates++
	if f.err != nil {
		return nil, f.err
	}
	return &session.Session{Name: name, Dir: workDir, Status: "running", RCURL: "https://claude.ai/code/xyz"}, nil
}
func (f *fakeManager) List() ([]session.Session, error) { return nil, nil }
func (f *fakeManager) Kill(string) error                { return nil }
func (f *fakeManager) IsRunning(string) bool            { return f.running }

// cbx setup and cbx activate used to spell out the session name and working
// directory separately, and disagreed on both. Whatever this package decides is
// now the single answer for either command.
func TestStartUsesOneNameAndOneWorkspace(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "master.state")
	root := t.TempDir()
	mgr := &fakeManager{}

	sess, err := StartIn(mgr, root)
	if err != nil {
		t.Fatalf("StartIn: %v", err)
	}

	if mgr.name != Name {
		t.Errorf("session name = %q, want %q", mgr.name, Name)
	}
	want := filepath.Join(root, Name)
	if mgr.workDir != want {
		t.Errorf("working dir = %q, want %q", mgr.workDir, want)
	}
	if sess.RCURL == "" {
		t.Error("Remote Control URL not passed back to the caller")
	}
}

func TestStartReportsSessionFailure(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "master.state")
	mgr := &fakeManager{err: errors.New("not logged in")}

	if _, err := StartIn(mgr, t.TempDir()); err == nil {
		t.Fatal("expected an error when the session fails to start")
	}
}

// Restarting a live master session drops the conversation the user is in the
// middle of and mints a URL that invalidates the one on their phone.
func TestStartLeavesARunningSessionAlone(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "master.state")
	mgr := &fakeManager{}

	if _, err := StartIn(mgr, t.TempDir()); err != nil {
		t.Fatalf("first start: %v", err)
	}
	mgr.running = true

	sess, err := StartIn(mgr, t.TempDir())
	if err != nil {
		t.Fatalf("second start: %v", err)
	}

	if mgr.creates != 1 {
		t.Errorf("Create called %d times, want 1 — the running session was restarted", mgr.creates)
	}
	if sess.Status != "already running" {
		t.Errorf("status = %q, want %q", sess.Status, "already running")
	}
	// Without this the caller has a live session it cannot tell the user how to
	// reach, which is the whole point of not restarting it.
	if sess.RCURL != "https://claude.ai/code/xyz" {
		t.Errorf("Remote Control URL = %q, want the URL from the first start", sess.RCURL)
	}
}

// The URL is scraped from the tmux pane once at startup and is otherwise
// unrecoverable, so it has to outlive the process that captured it.
func TestRemoteControlURLSurvivesTheProcess(t *testing.T) {
	statePath = filepath.Join(t.TempDir(), "master.state")

	if got := RemoteControlURL(); got != "" {
		t.Errorf("URL = %q before any session started, want empty", got)
	}
	if _, err := StartIn(&fakeManager{}, t.TempDir()); err != nil {
		t.Fatalf("StartIn: %v", err)
	}
	if got := RemoteControlURL(); got != "https://claude.ai/code/xyz" {
		t.Errorf("URL = %q after start, want the captured URL", got)
	}
}
