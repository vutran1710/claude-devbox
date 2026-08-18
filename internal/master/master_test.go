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
	err     error
}

func (f *fakeManager) Create(name, workDir string) (*session.Session, error) {
	f.name, f.workDir = name, workDir
	if f.err != nil {
		return nil, f.err
	}
	return &session.Session{Name: name, Dir: workDir, Status: "running", RCURL: "https://claude.ai/code/xyz"}, nil
}
func (f *fakeManager) List() ([]session.Session, error) { return nil, nil }
func (f *fakeManager) Kill(string) error                { return nil }
func (f *fakeManager) IsRunning(string) bool            { return false }

// cbx setup and cbx activate used to spell out the session name and working
// directory separately, and disagreed on both. Whatever this package decides is
// now the single answer for either command.
func TestStartUsesOneNameAndOneWorkspace(t *testing.T) {
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
	mgr := &fakeManager{err: errors.New("not logged in")}

	if _, err := StartIn(mgr, t.TempDir()); err == nil {
		t.Fatal("expected an error when the session fails to start")
	}
}
