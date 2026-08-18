// Package master owns the always-on session the user drives from their phone.
// Both cbx setup and cbx activate bring it up, so its name, its workspace, and
// its startup live here rather than being spelled out twice.
package master

import (
	"fmt"

	"github.com/vutran1710/claudebox/internal/session"
	"github.com/vutran1710/claudebox/internal/workspace"
)

// Name is the session name. It is also the directory under the workspace root,
// so the session comes up in /workspace/master.
const Name = "master"

// Start brings up the master session, replacing any session already running
// under that name.
func Start() (*session.Session, error) {
	return StartIn(session.NewTmuxManager(), workspace.DefaultRoot)
}

// StartIn brings up the master session with an explicit manager and workspace
// root, so callers can substitute both in tests.
func StartIn(mgr session.Manager, root string) (*session.Session, error) {
	if root == "" {
		root = workspace.DefaultRoot
	}
	dir, err := workspace.ResolveIn(root, Name, "")
	if err != nil {
		return nil, fmt.Errorf("resolve %s workspace: %w", Name, err)
	}
	sess, err := mgr.Create(Name, dir.Dir)
	if err != nil {
		return nil, fmt.Errorf("start %s session: %w", Name, err)
	}
	return sess, nil
}
