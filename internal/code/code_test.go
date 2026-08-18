package code

import (
	"strings"
	"testing"

	"github.com/vutran1710/claudebox/internal/master"
)

// cbx code goes straight to TmuxManager.Create, which opens by killing the
// session it is asked for. Left unguarded, cbx code master would destroy the
// running master session that cbx setup and cbx activate deliberately leave
// alone — dropping the conversation and invalidating the URL on the user's
// phone.
func TestMasterIsRoutedToItsOwner(t *testing.T) {
	isMaster, err := masterRequest(master.Name, "")
	if err != nil {
		t.Fatalf("masterRequest(%q): %v", master.Name, err)
	}
	if !isMaster {
		t.Errorf("cbx code %s is not routed through the master package — it would restart the running session", master.Name)
	}
}

func TestOrdinarySessionsAreUnaffected(t *testing.T) {
	for _, name := range []string{"my-app", "master-plan", "notmaster"} {
		isMaster, err := masterRequest(name, "owner/repo")
		if err != nil {
			t.Errorf("masterRequest(%q): unexpected error %v", name, err)
		}
		if isMaster {
			t.Errorf("%q was treated as the master session", name)
		}
	}
}

// Cloning into the master workspace while its session is live is incoherent:
// the session is left alone, so the clone would sit there unused.
func TestMasterRejectsARepo(t *testing.T) {
	_, err := masterRequest(master.Name, "owner/repo")
	if err == nil {
		t.Fatalf("cbx code %s --repo was accepted", master.Name)
	}
	if !strings.Contains(err.Error(), "cbx code") {
		t.Errorf("error does not point at the alternative:\n%s", err)
	}
}
