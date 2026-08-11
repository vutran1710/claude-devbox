package workspace

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

// stubOwnerSeams makes the process look like root with a resolvable owner and
// records every chown instead of performing it. Returns the recorded paths.
func stubOwnerSeams(t *testing.T, uid, gid string) map[string][2]int {
	t.Helper()
	origEuid, origLookup, origChown := geteuid, lookupUser, chownFn
	t.Cleanup(func() { geteuid, lookupUser, chownFn = origEuid, origLookup, origChown })

	chowned := map[string][2]int{}
	geteuid = func() int { return 0 }
	lookupUser = func(name string) (*user.User, error) {
		if name != Owner {
			t.Errorf("looked up user %q, want %q", name, Owner)
		}
		return &user.User{Uid: uid, Gid: gid}, nil
	}
	chownFn = func(path string, u, g int) error {
		chowned[path] = [2]int{u, g}
		return nil
	}
	return chowned
}

func TestResolveIn_FindExisting(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "my-app"), 0755)

	result, err := ResolveIn(root, "my-app", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "found" {
		t.Errorf("expected 'found', got %q", result.Status)
	}
}

func TestResolveIn_CreateNew(t *testing.T) {
	root := t.TempDir()

	result, err := ResolveIn(root, "new-project", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "created" {
		t.Errorf("expected 'created', got %q", result.Status)
	}
	if _, err := os.Stat(result.Dir); err != nil {
		t.Error("directory should exist")
	}
}

func TestResolveIn_FindClonedRepo(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "my-repo", ".git"), 0755)

	result, err := ResolveIn(root, "my-repo", "owner/my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "found" {
		t.Errorf("expected 'found', got %q", result.Status)
	}
}

func TestApplyOwner_ChownsEntireTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "deep"), 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	nested := filepath.Join(root, "sub", "file.txt")
	if err := os.WriteFile(nested, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	origChown := chownFn
	t.Cleanup(func() { chownFn = origChown })
	chowned := map[string][2]int{}
	chownFn = func(path string, u, g int) error {
		chowned[path] = [2]int{u, g}
		return nil
	}

	if err := applyOwner(root, 1000, 1000); err != nil {
		t.Fatalf("applyOwner: %v", err)
	}

	want := []string{root, filepath.Join(root, "sub"), filepath.Join(root, "sub", "deep"), nested}
	for _, p := range want {
		ids, ok := chowned[p]
		if !ok {
			t.Errorf("never chowned %s", p)
			continue
		}
		if ids != [2]int{1000, 1000} {
			t.Errorf("chowned %s to %v, want [1000 1000]", p, ids)
		}
	}
}

func TestOwnerIDs_SkipsWhenUnprivileged(t *testing.T) {
	origEuid := geteuid
	t.Cleanup(func() { geteuid = origEuid })
	geteuid = func() int { return 1000 }

	_, _, ok, err := ownerIDs(Owner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false when not root — an unprivileged cbx cannot chown")
	}
}

func TestOwnerIDs_ErrorsWhenUserMissing(t *testing.T) {
	origEuid, origLookup := geteuid, lookupUser
	t.Cleanup(func() { geteuid, lookupUser = origEuid, origLookup })
	geteuid = func() int { return 0 }
	lookupUser = func(string) (*user.User, error) { return nil, user.UnknownUserError("claude") }

	if _, _, _, err := ownerIDs(Owner); err == nil {
		t.Error("expected an error when the session user does not exist")
	}
}

// The bug: cbx serve runs as root, so the workspace it creates was owned by
// root, while sessions run as `claude` via su — leaving Claude unable to write.
func TestResolveIn_CreateNew_ChownsToSessionUser(t *testing.T) {
	root := t.TempDir()
	chowned := stubOwnerSeams(t, "1000", "1000")

	result, err := ResolveIn(root, "new-project", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := chowned[result.Dir]; !ok {
		t.Errorf("workspace %s was never chowned to %q — session user cannot write to it", result.Dir, Owner)
	}
}

func TestResolveIn_FindExisting_ChownsToSessionUser(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-app")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	chowned := stubOwnerSeams(t, "1000", "1000")

	if _, err := ResolveIn(root, "my-app", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := chowned[dir]; !ok {
		t.Errorf("pre-existing workspace %s was never chowned to %q", dir, Owner)
	}
}

func TestResolveIn_PropagatesOwnershipFailure(t *testing.T) {
	root := t.TempDir()
	origEuid, origLookup := geteuid, lookupUser
	t.Cleanup(func() { geteuid, lookupUser = origEuid, origLookup })
	geteuid = func() int { return 0 }
	lookupUser = func(string) (*user.User, error) { return nil, user.UnknownUserError("claude") }

	if _, err := ResolveIn(root, "new-project", ""); err == nil {
		t.Error("expected ResolveIn to fail loudly rather than return an unwritable workspace")
	}
}

func TestNormalizeRepoURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"owner/repo", "https://github.com/owner/repo.git"},
		{"https://github.com/a/b.git", "https://github.com/a/b.git"},
		{"git@github.com:a/b.git", "git@github.com:a/b.git"},
	}
	for _, tt := range tests {
		got := normalizeRepoURL(tt.input)
		if got != tt.want {
			t.Errorf("normalizeRepoURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsGitRepo(t *testing.T) {
	root := t.TempDir()
	if isGitRepo(root) {
		t.Error("empty dir should not be a git repo")
	}
	os.MkdirAll(filepath.Join(root, ".git"), 0755)
	if !isGitRepo(root) {
		t.Error("dir with .git should be a git repo")
	}
}
