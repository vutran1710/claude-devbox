package main

import (
	"fmt"
	"os"
	"syscall"
)

// daemonAttr builds the process attributes for a detached cbx serve daemon.
//
// The daemon must not share the launching shell's stdio. Started over SSH it
// would otherwise inherit that session's pipe and die on its first log write
// once the pipe closed — leaving a tunnel pointing at nothing. Output goes to
// logPath instead, and Setsid puts the daemon in its own session so a
// departing terminal cannot SIGHUP it.
//
// The returned func closes the parent's copies of the descriptors; call it
// once the child has been started.
func daemonAttr(logPath string) (*os.ProcAttr, func(), error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open daemon log %s: %w", logPath, err)
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		logFile.Close()
		return nil, nil, fmt.Errorf("open %s: %w", os.DevNull, err)
	}

	attr := &os.ProcAttr{
		Dir:   "/",
		Env:   os.Environ(),
		Files: []*os.File{devNull, logFile, logFile},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}
	return attr, func() { devNull.Close(); logFile.Close() }, nil
}
