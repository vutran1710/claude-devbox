package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func Run(ctx context.Context, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, err
}

func RunTimeout(timeout time.Duration, name string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Run(ctx, name, args...)
}

const FullPATH = "/usr/sbin:/root/.local/bin:/root/.npm-global/bin:/root/.cargo/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
const ShellPATH = "PATH=" + FullPATH

func RunShell(ctx context.Context, script string) (Result, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Env = append(cmd.Environ(), ShellPATH, "HOME=/root")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, err
}

func RunShellTimeout(timeout time.Duration, script string) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return RunShell(ctx, script)
}

// Which reports whether a binary is on the provisioning PATH.
//
// It resolves against FullPATH explicitly rather than the process environment.
// An earlier version set os.Setenv("PATH", FullPATH) in an init() so LookPath
// would find these directories — but that overwrote the PATH of every process
// importing this package, which on a developer's Mac silently erased
// /opt/homebrew/bin and made tmux and gh invisible to unrelated code.
func Which(binary string) bool {
	for _, dir := range filepath.SplitList(FullPATH) {
		info, err := os.Stat(filepath.Join(dir, binary))
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

func FileExists(path string) bool {
	_, err := RunTimeout(5*time.Second, "test", "-f", path)
	return err == nil
}

func ProcessRunning(pattern string) bool {
	res, _ := RunTimeout(5*time.Second, "pgrep", "-f", pattern)
	return res.ExitCode == 0
}

func SystemdActive(service string) bool {
	res, _ := RunTimeout(5*time.Second, "systemctl", "is-active", service)
	return res.Stdout == "active\n" || res.Stdout == "active"
}

func GetPublicIP() string {
	res, err := RunTimeout(10*time.Second, "curl", "-sf", "https://ifconfig.me")
	if err != nil {
		r2, _ := RunShellTimeout(5*time.Second, "hostname -I | awk '{print $1}'")
		return fmt.Sprintf("%s", r2.Stdout)
	}
	return res.Stdout
}
