// Command cbx manages Claude Code sessions on the machine it runs on.
//
// It is non-interactive by contract. Its caller is the master Claude session,
// which has a shell but no terminal and no human: nothing here reads stdin, no
// command prompts, the exit code is the result, and output is one
// tab-separated fact per line so it can be cut without parsing a TUI.
//
// It knows nothing about SSH or remote machines. Provisioning a box is
// cbx-setuptool's job.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vutran1710/claudebox/internal/cbx"
	"github.com/vutran1710/claudebox/internal/store"
)

var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cbx",
		Short: "Manage Claude Code sessions on this machine",
		Long: `cbx manages Claude Code sessions on the machine it runs on.

It is non-interactive: it never reads stdin, never prompts, and prints one
tab-separated fact per line. The exit code is the result. This is deliberate —
it is driven by the master Claude session, which has no terminal.

It does not know about SSH or other machines. Setting up a box is
cbx-setuptool's job.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCmd(), lsCmd(), killCmd(), resumeCmd(), exportCmd())
	return root
}

// withApp opens the session database and runs fn. Every command needs it, and
// none of them should each remember to close it.
func withApp(fn func(*cbx.App) error) error {
	st, err := store.Open(store.DefaultPath())
	if err != nil {
		return err
	}
	defer st.Close()
	return fn(cbx.New(st))
}

func newCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Start a Claude Code session",
		Long: `Starts a detached Claude Code session and enables Remote Control,
printing the URL to open it from a phone.

The name maps to a directory under the workspace root: an existing directory is
used as-is, --repo clones into a new one, and otherwise an empty directory is
created.

Refuses if a session of that name is already running — use resume to get its
URL, or kill it first.`,
		Example: "  cbx new my-app\n  cbx new my-app --repo owner/repo",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(a *cbx.App) error { return a.New(args[0], repo) })
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Git repo to clone (owner/repo or a full URL)")
	return cmd
}

func lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List sessions and whether they are running",
		Long: `Lists every recorded session as: name, status, directory, URL.

Status comes from reconciling the database against tmux, so a session that was
started and has since died reports "stopped" rather than disappearing. Sessions
started outside cbx are listed too.`,
		Example: "  cbx ls\n  cbx ls | awk -F'\\t' '$2==\"running\"'",
		Args:    cobra.NoArgs,
		RunE:    func(_ *cobra.Command, _ []string) error { return withApp(func(a *cbx.App) error { return a.List() }) },
	}
}

func killCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "kill <name>",
		Short:   "Stop a session and forget it",
		Long:    "Stops the session and removes its record. Killing a session that is not running succeeds — the intent is that it be gone.",
		Example: "  cbx kill my-app",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(a *cbx.App) error { return a.Kill(args[0]) })
		},
	}
}

func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <name>",
		Short: "Print how to reach a running session",
		Long: `Prints a running session's directory, Remote Control URL, and the tmux
command to attach to it.

It does not attach. Attaching needs a terminal and cbx never assumes it has
one; run the printed command yourself if you are at one.`,
		Example: "  cbx resume my-app",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(a *cbx.App) error { return a.Resume(args[0]) })
		},
	}
}

func exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export <skills|rules|db>",
		Short: "Print what this box has, to stdout",
		Long: `Writes part of this box's configuration to stdout so it can be read,
diffed, or saved.

  skills   installed skills, with their descriptions
  rules    CLAUDE.md and settings that shape sessions
  db       the session database, tab-separated

This is the mirror of cbx-setuptool, which pushes config from a laptop to a
box. Export reads out from the box, so the master session can answer what it
has and what it has been doing without anyone opening an SSH connection.`,
		Example: "  cbx export skills\n  cbx export db > sessions.tsv",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(a *cbx.App) error { return a.Export(args[0]) })
		},
	}
}
