// Package master owns the always-on session the user drives from their phone.
// Both cbx setup and cbx activate bring it up, so its name, its workspace, its
// startup, and the record of what it is are here rather than spelled out twice.
package master

import (
	"fmt"
	"os"
	"strings"

	"github.com/vutran1710/claudebox/internal/session"
	"github.com/vutran1710/claudebox/internal/workspace"
)

// Name is the session name. It is also the directory under the workspace root,
// so the session comes up in /workspace/master.
const Name = "master"

// statePath records the running session's Remote Control URL. tmux stays the
// authority on whether the session is alive — this only caches the URL, which
// is scraped once at startup and otherwise unrecoverable. Kept in a var so
// tests can point it somewhere harmless.
var statePath = "/tmp/cbx-master.state"

// Start brings up the master session, or hands back the one already running.
func Start() (*session.Session, error) {
	return StartIn(session.NewTmuxManager(), workspace.DefaultRoot)
}

// StartIn brings up the master session with an explicit manager and workspace
// root, so callers can substitute both in tests.
//
// A master session already running is left alone: restarting it would drop the
// conversation the user is in the middle of and mint a Remote Control URL that
// invalidates the one they already have on their phone.
func StartIn(mgr session.Manager, root string) (*session.Session, error) {
	if root == "" {
		root = workspace.DefaultRoot
	}

	if mgr.IsRunning(Name) {
		return &session.Session{
			Name:   Name,
			Status: "already running",
			RCURL:  RemoteControlURL(),
		}, nil
	}

	dir, err := workspace.ResolveIn(root, Name, "")
	if err != nil {
		return nil, fmt.Errorf("resolve %s workspace: %w", Name, err)
	}
	sess, err := mgr.Create(Name, dir.Dir)
	if err != nil {
		return nil, fmt.Errorf("start %s session: %w", Name, err)
	}
	saveState(sess.RCURL)
	return sess, nil
}

// RemoteControlURL is the URL of the running master session, or "" if the
// session has never been started or its URL was never captured.
func RemoteControlURL() string {
	data, _ := os.ReadFile(statePath)
	for _, line := range strings.Split(string(data), "\n") {
		if url, ok := strings.CutPrefix(line, "remote_control="); ok {
			return url
		}
	}
	return ""
}

// saveState is best-effort: a session that came up but could not be recorded is
// still a working session, and the URL is on screen either way.
func saveState(rcURL string) {
	if rcURL == "" {
		return
	}
	if os.WriteFile(statePath, []byte("remote_control="+rcURL+"\n"), 0644) != nil {
		return
	}
	// cbx setup writes this as root; cbx activate reads and rewrites it as the
	// claude user.
	workspace.EnsureOwner(statePath)
}
