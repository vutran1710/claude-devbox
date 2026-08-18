package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vutran1710/claudebox/internal/shell"
)

const DefaultRoot = "/workspace"

// Owner is the system user that must own workspace directories. Sessions run
// as this user (see session.ClaudeUser), so anything cbx creates while running
// as root has to be handed over or the session cannot write to it.
const Owner = "claude"

// Seams for the OS calls involved in changing ownership, swapped in tests.
var (
	geteuid    = os.Geteuid
	lookupUser = user.Lookup
	chownFn    = os.Lchown
)

// ownerIDs resolves the uid/gid that workspace directories should belong to.
// ok is false when the process is unprivileged: only root can hand a directory
// to another user, and an unprivileged cbx already creates directories owned by
// whoever runs it.
func ownerIDs(owner string) (uid, gid int, ok bool, err error) {
	if geteuid() != 0 {
		return 0, 0, false, nil
	}
	u, err := lookupUser(owner)
	if err != nil {
		return 0, 0, false, fmt.Errorf("lookup user %s: %w", owner, err)
	}
	if uid, err = strconv.Atoi(u.Uid); err != nil {
		return 0, 0, false, fmt.Errorf("uid for %s: %w", owner, err)
	}
	if gid, err = strconv.Atoi(u.Gid); err != nil {
		return 0, 0, false, fmt.Errorf("gid for %s: %w", owner, err)
	}
	return uid, gid, true, nil
}

// applyOwner chowns dir and everything beneath it. Symlinks are retargeted
// themselves rather than followed, so a link inside a cloned repo cannot
// redirect the chown outside the workspace.
func applyOwner(dir string, uid, gid int) error {
	return filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return chownFn(path, uid, gid)
	})
}

// EnsureOwner hands path to the user that sessions run as. Without this a
// workspace created by the root-owned cbx serve daemon is read-only to the
// session working in it. path may be a single file as well as a directory.
func EnsureOwner(path string) error {
	uid, gid, ok, err := ownerIDs(Owner)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := applyOwner(path, uid, gid); err != nil {
		return fmt.Errorf("failed to give %s to %s: %w", path, Owner, err)
	}
	return nil
}

type ResolveResult struct {
	Dir    string `json:"dir"`
	Status string `json:"status"` // "cloned", "found", "created"
}

// Resolve finds or creates a project directory.
// If repo is provided, clones it. Otherwise finds or creates the dir with git init.
func Resolve(name, repo string) (*ResolveResult, error) {
	return ResolveIn(DefaultRoot, name, repo)
}

// ResolveIn finds or creates a project directory in a given root.
func ResolveIn(root, name, repo string) (*ResolveResult, error) {
	dir := filepath.Join(root, name)

	// If repo provided, find or clone
	if repo != "" {
		status := "found"
		if !isGitRepo(dir) {
			url := normalizeRepoURL(repo)
			_, err := shell.RunShellTimeout(2*time.Minute, fmt.Sprintf(`git clone %s %s`, url, dir))
			if err != nil {
				return nil, fmt.Errorf("clone failed: %w", err)
			}
			status = "cloned"
		}
		if err := EnsureOwner(dir); err != nil {
			return nil, err
		}
		return &ResolveResult{Dir: dir, Status: status}, nil
	}

	// No repo — find existing or create new
	status := "found"
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		shell.RunShellTimeout(10*time.Second, fmt.Sprintf(`cd %s && git init`, dir))
		status = "created"
	}
	if err := EnsureOwner(dir); err != nil {
		return nil, err
	}
	return &ResolveResult{Dir: dir, Status: status}, nil
}

// normalizeRepoURL converts shorthand "owner/repo" to full URL.
// Passes through full URLs (https://, git@) as-is.
func normalizeRepoURL(repo string) string {
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	return fmt.Sprintf("https://github.com/%s.git", repo)
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
