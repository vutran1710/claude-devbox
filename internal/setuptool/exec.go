package setuptool

import "os/exec"

// exec_ssh builds an ssh command whose stdin the caller supplies. Separate
// from Run so a token can be piped in without appearing in argv.
func exec_ssh(t Target, script string) *exec.Cmd {
	return exec.Command("ssh", sshArgs(t, script)...)
}
