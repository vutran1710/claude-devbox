package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vutran1710/claudebox/internal/activate"
	"github.com/vutran1710/claudebox/internal/code"
	"github.com/vutran1710/claudebox/internal/remote"
	"github.com/vutran1710/claudebox/internal/serve"
	"github.com/vutran1710/claudebox/internal/setup"
	"github.com/vutran1710/claudebox/internal/status"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "cbx",
		Short:   "ClaudeBox CLI",
		Version: version,
	}

	root.AddCommand(setupCmd())
	root.AddCommand(activateCmd())
	root.AddCommand(codeCmd())
	root.AddCommand(serveCmd())
	root.AddCommand(showCmd())
	root.AddCommand(statusCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func setupCmd() *cobra.Command {
	var ip, user string
	var assumeYes bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install tools, authenticate Claude, start VNC",
		Long: `Installs all dev tools, authenticates Claude Code via OAuth,
and starts VNC + Chrome.

With --ip, cbx runs the same setup on that machine over SSH instead of this
one, passing your terminal through so the Claude login works as if you were
sitting at the box. cbx must already be installed there — a droplet deployed
by ClaudeBox has it from cloud-init.

  cbx setup                        # this machine (run as root)
  cbx setup --ip 203.0.113.9       # a remote box, as root
  cbx setup --ip 203.0.113.9 -y    # skip the confirmation prompt

Note --ip is not unattended: the Claude subscription login is an interactive
browser OAuth, so it still waits for you. -y only skips cbx's own prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ip == "" {
				return setup.Run()
			}
			target, err := remote.NewTarget(ip, user)
			if err != nil {
				return err
			}
			if !assumeYes && !confirm(fmt.Sprintf("Run cbx setup on %s?", target)) {
				return fmt.Errorf("cancelled")
			}
			return remote.Run(target, "cbx setup")
		},
	}

	cmd.Flags().StringVar(&ip, "ip", "", "Run setup on this host over SSH instead of locally")
	cmd.Flags().StringVar(&user, "user", "root", "SSH user for --ip")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

// confirm asks for a y/N on stdin. A non-tty stdin (a pipe, a CI job) reads as
// no rather than hanging or silently proceeding.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	var answer string
	if _, err := fmt.Scanln(&answer); err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func activateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate",
		Short: "Start the master session",
		Long: `Brings up the master session — the same always-on session cbx setup
starts — with remote-control and dangerously-skip-permissions enabled, and
prints its Remote Control URL. A master session already running is left alone
and its URL reported. Run as claude user.

  ssh -t claude@<host> 'cbx activate'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return activate.Run()
		},
	}
}

func codeCmd() *cobra.Command {
	var repo string
	var headless bool

	cmd := &cobra.Command{
		Use:   "code <name>",
		Short: "Spawn a new Claude Code session",
		Long: `Spawn a new Claude Code tmux session with remote-control and
dangerously-skip-permissions enabled.

The name maps to a directory in /workspace:
  - If the directory exists, opens it
  - If --repo is set, clones the repo there
  - Otherwise, creates a new directory with git init

The master session is owned by cbx: 'cbx code master' reports the running
session rather than restarting it.

Examples:
  cbx code hello-world                     # find or create /workspace/hello-world
  cbx code my-app --repo owner/repo        # clone GitHub repo
  cbx code my-app --repo https://...       # clone any git URL
  cbx code my-app --headless               # non-interactive mode`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return code.Run(args[0], repo, headless)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Git repo to clone (owner/repo or full URL)")
	cmd.Flags().BoolVar(&headless, "headless", false, "Non-interactive mode (no TUI)")
	return cmd
}

func serveCmd() *cobra.Command {
	var port int
	var stop bool
	var daemon bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the session management API daemon",
		Long: `Runs an HTTP API for managing Claude Code sessions.

  cbx serve                  # start in foreground
  cbx serve -d               # start as background daemon
  cbx serve --stop           # stop the daemon

API endpoints:
  POST   /sessions           Create session { name, repo? }
  GET    /sessions           List sessions
  DELETE /sessions/{name}    Kill session
  GET    /health             Health check`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stop {
				if err := serve.Stop(); err != nil {
					return err
				}
				fmt.Println("cbx serve stopped")
				return nil
			}
			if daemon {
				return startDaemon(port)
			}
			return serve.New(port).Start()
		},
	}

	cmd.Flags().IntVarP(&port, "port", "P", serve.DefaultPort, "Port to listen on")
	cmd.Flags().BoolVar(&stop, "stop", false, "Stop the daemon")
	cmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Run as background daemon")
	return cmd
}

func startDaemon(port int) error {
	// Re-exec ourselves in the background
	exe, _ := os.Executable()
	args := []string{exe, "serve", "--port", fmt.Sprintf("%d", port)}

	attr, closeFiles, err := daemonAttr(serve.LogFile)
	if err != nil {
		return err
	}
	defer closeFiles()

	proc, err := os.StartProcess(exe, args, attr)
	if err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}
	proc.Release()
	fmt.Printf("cbx serve started (port %d), logging to %s\n", port, serve.LogFile)
	return nil
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [item]",
		Short: "Show configuration values",
		Long: `Show configuration values.

  cbx show api-key           Show the API key for cbx serve`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "api-key":
				key := serve.GetAPIKey()
				if key == "" {
					return fmt.Errorf("no API key found — run 'cbx serve' first")
				}
				fmt.Println(key)
				return nil
			default:
				return fmt.Errorf("unknown item: %s (available: api-key)", args[0])
			}
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all services",
		Run: func(cmd *cobra.Command, args []string) {
			status.Run()
		},
	}
}
