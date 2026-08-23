// Package cbx implements the session commands. It is separated from the
// cobra wiring so the behaviour can be tested without a CLI harness.
//
// Everything here is non-interactive by contract: nothing reads stdin, no
// command prompts, and output is one fact per line. The caller is the master
// Claude session, which has a shell but no terminal and no human.
package cbx

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vutran1710/claudebox/internal/store"
	"github.com/vutran1710/claudebox/internal/tmux"
)

// App holds the dependencies the commands need. Injected so tests drive real
// tmux and a temp database without touching the user's own.
type App struct {
	Tmux    *tmux.Client
	Store   *store.Store
	Root    string    // where project directories live
	Out     io.Writer //
	Err     io.Writer
	Timeout time.Duration // how long to wait for a Remote Control URL
}

// DefaultRoot is where projects live. /workspace on a box that has one,
// otherwise ~/workspace — a laptop has no writable /workspace, and cbx must be
// runnable there.
func DefaultRoot() string {
	if info, err := os.Stat("/workspace"); err == nil && info.IsDir() {
		return "/workspace"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/workspace"
	}
	return filepath.Join(home, "workspace")
}

// New builds an App with the real dependencies.
func New(st *store.Store) *App {
	return &App{
		Tmux:    tmux.New(),
		Store:   st,
		Root:    DefaultRoot(),
		Out:     os.Stdout,
		Err:     os.Stderr,
		Timeout: 60 * time.Second,
	}
}

// New starts a session called name. repo, if given, is cloned into the
// project directory; otherwise an existing directory is used or an empty one
// created.
func (a *App) New(name, repo string) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}
	if a.Tmux.Has(name) {
		return fmt.Errorf("session %q is already running — `cbx resume %s` to attach, or `cbx kill %s` first", name, name, name)
	}

	dir := filepath.Join(a.Root, name)
	if err := a.prepareDir(dir, repo); err != nil {
		return err
	}
	if err := a.Tmux.Start(name, dir); err != nil {
		return err
	}

	// A session with no Remote Control URL is still usable over tmux, so
	// neither of these undoes the start — but say which of the two happened.
	// An unauthenticated box never produces a URL, and waiting the full
	// timeout to report a bare "none appeared" tells nobody why.
	var url string
	var urlErr error
	if !a.Tmux.LoggedIn() {
		urlErr = fmt.Errorf("Claude Code is not signed in on this machine, so the session has no Remote Control URL — sign in with `cbx-setuptool setup`")
	} else {
		url, urlErr = a.Tmux.EnableRemoteControl(name, a.Timeout)
	}

	if err := a.Store.Put(store.Session{Name: name, Dir: dir, Repo: repo, RCURL: url}); err != nil {
		return err
	}

	fmt.Fprintf(a.Out, "name\t%s\n", name)
	fmt.Fprintf(a.Out, "dir\t%s\n", dir)
	if url != "" {
		fmt.Fprintf(a.Out, "url\t%s\n", url)
	}
	if urlErr != nil {
		fmt.Fprintf(a.Err, "warning: session started but no Remote Control URL: %v\n", urlErr)
	}
	return nil
}

// prepareDir makes the project directory ready: cloned, existing, or new.
func (a *App) prepareDir(dir, repo string) error {
	if _, err := os.Stat(dir); err == nil {
		if repo != "" {
			// Refuse rather than clone over someone's work.
			if entries, _ := os.ReadDir(dir); len(entries) > 0 {
				return fmt.Errorf("%s already exists and is not empty — remove it or omit --repo", dir)
			}
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if repo == "" {
		return nil
	}
	url, err := repoURL(repo)
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	// exec.Command, not a shell string. Shell-quoting makes a value safe for
	// the shell but not for argv: git reads a leading dash as an option, and
	// `git clone --upload-pack=...` runs an arbitrary command. "--" ends
	// option parsing, and going through exec directly means there is no shell
	// to quote for in the first place.
	out, err := exec.Command("git", "clone", "--", url, dir).CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("clone %s: %w: %s", repo, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// repoURL expands owner/repo shorthand and rejects anything git would read as
// an option rather than a repository.
func repoURL(repo string) (string, error) {
	if strings.HasPrefix(repo, "-") {
		return "", fmt.Errorf("invalid repo %q: leading dash would be read as a git option", repo)
	}
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@") {
		return repo, nil
	}
	// Shorthand must look like owner/repo and nothing else.
	if !shorthand.MatchString(repo) {
		return "", fmt.Errorf("invalid repo %q: expected owner/repo or a full git URL", repo)
	}
	return "https://github.com/" + repo + ".git", nil
}

var shorthand = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// List prints every recorded session and whether it is running. Reconciling
// the store against tmux is the point: a row can outlive its process.
func (a *App) List() error {
	recorded, err := a.Store.List()
	if err != nil {
		return err
	}
	running, err := a.Tmux.List()
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, n := range running {
		live[n] = true
	}

	seen := map[string]bool{}
	for _, s := range recorded {
		seen[s.Name] = true
		fmt.Fprintf(a.Out, "%s\t%s\t%s\t%s\n", s.Name, statusOf(live[s.Name]), s.Dir, s.RCURL)
	}
	// A session started outside cbx is still a session; report it rather than
	// pretend the store is the whole truth.
	for _, n := range running {
		if !seen[n] {
			fmt.Fprintf(a.Out, "%s\t%s\t%s\t%s\n", n, "running", "", "")
		}
	}
	return nil
}

func statusOf(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

// Kill stops a session and forgets it.
func (a *App) Kill(name string) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}
	if err := a.Tmux.Kill(name); err != nil {
		return err
	}
	return a.Store.Delete(name)
}

// Resume prints how to attach to a running session, and its URL. It does not
// attach: attaching needs a terminal, and cbx never assumes it has one.
func (a *App) Resume(name string) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}
	if !a.Tmux.Has(name) {
		rec, _ := a.Store.Get(name)
		if rec != nil {
			return fmt.Errorf("session %q is recorded but not running — `cbx new %s` to start it again", name, name)
		}
		return fmt.Errorf("no session named %q", name)
	}
	rec, err := a.Store.Get(name)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "name\t%s\n", name)
	if rec != nil {
		fmt.Fprintf(a.Out, "dir\t%s\n", rec.Dir)
		if rec.RCURL != "" {
			fmt.Fprintf(a.Out, "url\t%s\n", rec.RCURL)
		}
	}
	fmt.Fprintf(a.Out, "attach\ttmux attach -t %s\n", name)
	return nil
}
