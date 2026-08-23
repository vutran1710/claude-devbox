// Package setuptool provisions a remote machine from your laptop.
//
// Everything here drives a box over SSH. It never runs on the box — that is
// cbx's half of the split. The two never share a process, so this package is
// free to be interactive and cbx is free to assume it never is.
package setuptool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrUnsafeTarget is returned for a host or user ssh could read as an option.
// ssh has no "--" sentinel, so a value beginning with "-" becomes a flag —
// -oProxyCommand=<cmd> in that position executes <cmd> locally.
var ErrUnsafeTarget = errors.New("unsafe ssh target")

// Target is a machine to provision.
type Target struct {
	User string
	Host string
}

func (t Target) String() string { return t.User + "@" + t.Host }

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

// validate restricts to a conservative set. ':' is excluded so a value can
// never split an scp destination, which also rules out IPv6 literals — cbx
// deploys IPv4 droplets.
func validate(field, v string) error {
	if v == "" {
		return fmt.Errorf("%s is empty: %w", field, ErrUnsafeTarget)
	}
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("%s %q starts with '-' and ssh would read it as an option: %w", field, v, ErrUnsafeTarget)
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

func sshArgs(t Target, extra ...string) []string {
	return append([]string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		t.String(),
	}, extra...)
}

// Run executes a script on the target and returns its combined output.
func Run(t Target, script string) (string, error) {
	out, err := exec.Command("ssh", sshArgs(t, script)...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w: %s", t, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Interactive runs a command on the target with the caller's terminal attached.
// The Claude login needs this: the sign-in URL has to reach the person running
// the tool, and the code they paste has to reach the box.
func Interactive(t Target, command string) error {
	args := append([]string{"-t",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		t.String()}, command)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// Upload copies a local file to the target. Uses SFTP (-s) rather than the
// legacy SCP protocol: under legacy SCP the remote path is expanded by the
// remote login shell, which turns a filename into a command injection.
func Upload(t Target, localPath, remotePath string) error {
	if strings.HasPrefix(localPath, "-") || strings.HasPrefix(remotePath, "-") {
		return fmt.Errorf("path starting with '-' would be read as an scp option: %w", ErrUnsafeTarget)
	}
	dest := fmt.Sprintf("%s@%s:%s", t.User, t.Host, remotePath)
	out, err := exec.Command("scp", "-s",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		localPath, dest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("upload %s to %s: %w: %s", localPath, t, err, strings.TrimSpace(string(out)))
	}
	return nil
}
