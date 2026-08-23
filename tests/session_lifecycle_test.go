package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vutran1710/claudebox/internal/store"
	"github.com/vutran1710/claudebox/internal/tmux"
)

// Integration coverage for the composition cbx actually performs: start a
// session with tmux, record it in the store, reconcile the two, kill it, and
// confirm the record and the process agree at every step.
//
// Both packages pass their own unit tests in isolation; this is the seam
// between them, which is where the last refactor's worst bugs lived. It drives
// real tmux and a real SQLite file on this machine — no fakes.

func liveClient(t *testing.T) *tmux.Client {
	t.Helper()
	if _, err := tmux.Shell("command -v tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	return tmux.New().WithCommand("sleep 300")
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionLifecycleAcrossStoreAndTmux(t *testing.T) {
	c := liveClient(t)
	db := openStore(t)
	name := fmt.Sprintf("cbxint-%d", os.Getpid())
	dir := t.TempDir()
	t.Cleanup(func() { c.Kill(name) })

	// Start, and record what was started.
	if err := c.Start(name, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := db.Put(store.Session{Name: name, Dir: dir, Repo: "owner/repo"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// The record and the process agree.
	rec, err := db.Get(name)
	if err != nil || rec == nil {
		t.Fatalf("Get: %v %v", rec, err)
	}
	if rec.Dir != dir {
		t.Errorf("recorded dir = %q, want %q", rec.Dir, dir)
	}
	if !c.Has(name) {
		t.Fatal("tmux does not have the session that was just started")
	}

	// Reconciliation: a recorded session that is running must be reported
	// running. This is what `cbx ls` does.
	running, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !contains(running, name) {
		t.Errorf("tmux list %v missing %q", running, name)
	}

	// Kill the process but leave the record: the store must now describe a
	// session that is gone, which is the state tmux alone could never express.
	if err := c.Kill(name); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	rec, _ = db.Get(name)
	if rec == nil {
		t.Fatal("record vanished when the process died — the store is not durable")
	}
	if c.Has(name) {
		t.Fatal("tmux still has a killed session")
	}

	// And forgetting it removes the record.
	if err := db.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rec, _ := db.Get(name); rec != nil {
		t.Error("record survived Delete")
	}
}

// The URL is the reason the store exists: it is produced once, at session
// start, and cannot be recovered from tmux afterwards.
func TestRecordedURLOutlivesTheTmuxServer(t *testing.T) {
	c := liveClient(t)
	db := openStore(t)
	name := fmt.Sprintf("cbxint-url-%d", os.Getpid())
	url := "https://claude.ai/code/session_01IntegrationFixture"
	t.Cleanup(func() { c.Kill(name) })

	if err := c.Start(name, t.TempDir()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	db.Put(store.Session{Name: name, Dir: t.TempDir(), RCURL: url})

	// Killing the session is the closest safe analogue to the server restart
	// that lost a real URL — killing the whole server would take the
	// developer's own sessions with it.
	c.Kill(name)

	rec, err := db.Get(name)
	if err != nil || rec == nil {
		t.Fatalf("Get after kill: %v %v", rec, err)
	}
	if rec.RCURL != url {
		t.Errorf("RCURL = %q, want %q — the URL was lost with the session", rec.RCURL, url)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return strings.Contains(strings.Join(ss, "\n"), s)
}
