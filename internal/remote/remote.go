// Package remote runs a cbx subcommand on another machine over SSH.
//
// cbx is installed on every box it deploys, so the simplest way to drive a
// remote box is to run the same command there rather than re-implement each
// step as a series of remote shell calls. The caller's terminal is passed
// straight through, which is what makes the interactive Claude Code login
// work: `cbx setup --ip <host>` behaves exactly like sitting at that box.
package remote

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrUnsafeTarget is returned for a host or user that could be mistaken for an
// ssh option. ssh has no "--" sentinel to end option parsing, so a value
// beginning with "-" would be read as a flag rather than a destination —
// -oProxyCommand=... being the obvious way to turn that into code execution.
var ErrUnsafeTarget = errors.New("unsafe ssh target")

// Target is a machine reachable over SSH.
type Target struct {
	User string
	Host string
}

func (t Target) String() string { return t.User + "@" + t.Host }

// NewTarget validates and builds a Target. user may be empty, in which case
// root is assumed — every droplet cbx deploys registers root.
func NewTarget(host, user string) (Target, error) {
	if user == "" {
		user = "root"
	}
	if err := validate("host", host); err != nil {
		return Target{}, err
	}
	if err := validate("user", user); err != nil {
		return Target{}, err
	}
	return Target{User: user, Host: host}, nil
}

// validate rejects anything ssh could read as an option, and anything outside
// a conservative character set. ':' is excluded so a value can never split a
// scp-style destination. This rules out IPv6 literals, which cbx does not use.
func validate(field, v string) error {
	if v == "" {
		return fmt.Errorf("%s is empty: %w", field, ErrUnsafeTarget)
	}
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("%s %q starts with '-' and would be read as an ssh option: %w", field, v, ErrUnsafeTarget)
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '_':
		default:
			return fmt.Errorf("%s %q contains %q: %w", field, v, r, ErrUnsafeTarget)
		}
	}
	return nil
}

// SSHArgs builds the argv for running remoteCmd on t. Split out from Run so a
// test can assert the command line without opening a connection.
func SSHArgs(t Target, remoteCmd string) []string {
	return []string{
		"-t", // allocate a tty: the Claude login is interactive
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		t.String(),
		remoteCmd,
	}
}

// Run executes remoteCmd on t with the caller's stdin, stdout and stderr
// attached, and returns when it exits.
func Run(t Target, remoteCmd string) error {
	cmd := exec.Command("ssh", SSHArgs(t, remoteCmd)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", t, err)
	}
	return nil
}
