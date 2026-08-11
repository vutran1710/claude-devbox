package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bug: startDaemon handed the child the parent's own stdout/stderr. Started
// over SSH, the daemon inherited the SSH pipe and died on its first log write
// once that pipe closed.
func TestDaemonAttr_DoesNotInheritParentStdio(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "cbx-serve.log")

	attr, closeFiles, err := daemonAttr(logPath)
	if err != nil {
		t.Fatalf("daemonAttr: %v", err)
	}
	defer closeFiles()

	names := []string{"stdin", "stdout", "stderr"}
	parent := []*os.File{os.Stdin, os.Stdout, os.Stderr}
	for i, f := range attr.Files {
		for _, p := range parent {
			if f == p {
				t.Errorf("child %s is the parent's %s — daemon dies when the parent's pipe closes", names[i], p.Name())
			}
		}
	}
}

func TestDaemonAttr_WritesStdoutAndStderrToLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "cbx-serve.log")

	attr, closeFiles, err := daemonAttr(logPath)
	if err != nil {
		t.Fatalf("daemonAttr: %v", err)
	}
	defer closeFiles()

	if _, err := attr.Files[1].WriteString("from stdout\n"); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if _, err := attr.Files[2].WriteString("from stderr\n"); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, want := range []string{"from stdout", "from stderr"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log %q missing %q", data, want)
		}
	}
}

func TestDaemonAttr_AppendsToExistingLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "cbx-serve.log")
	if err := os.WriteFile(logPath, []byte("earlier run\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	attr, closeFiles, err := daemonAttr(logPath)
	if err != nil {
		t.Fatalf("daemonAttr: %v", err)
	}
	defer closeFiles()

	if _, err := attr.Files[1].WriteString("later run\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "earlier run") {
		t.Errorf("restart truncated the log, losing prior output: %q", data)
	}
	if !strings.Contains(string(data), "later run") {
		t.Errorf("log %q missing the new output", data)
	}
}

// Without a new session the daemon keeps the launching terminal as its
// controlling terminal and takes SIGHUP when that terminal goes away.
func TestDaemonAttr_DetachesIntoNewSession(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "cbx-serve.log")

	attr, closeFiles, err := daemonAttr(logPath)
	if err != nil {
		t.Fatalf("daemonAttr: %v", err)
	}
	defer closeFiles()

	if attr.Sys == nil {
		t.Fatal("Sys is nil — no SysProcAttr, so Setsid is never requested")
	}
	if !attr.Sys.Setsid {
		t.Error("Setsid is false — daemon stays in the launching terminal's session and dies on SIGHUP")
	}
}

func TestDaemonAttr_ErrorsWhenLogUnwritable(t *testing.T) {
	unwritable := filepath.Join(t.TempDir(), "no-such-dir", "cbx-serve.log")

	if _, _, err := daemonAttr(unwritable); err == nil {
		t.Error("expected an error when the log file cannot be opened")
	}
}
