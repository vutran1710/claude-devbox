package setuptool

import (
	"fmt"
	"os"
	"strings"
)

// ToolAuth describes how one CLI is authenticated with a token.
//
// Each of these tools reads a token from somewhere different, and getting it
// wrong is silent: the tool installs, reports success, and only fails later
// when a session tries to use it. Keeping the recipes in one table makes the
// differences visible instead of scattered through provisioning scripts.
type ToolAuth struct {
	Name string
	// Where to get a token, shown to the person running the tool.
	TokenURL string
	// Help is a one-line description of what the token is for.
	Help string
	// login builds the remote command that consumes the token on stdin.
	// Taking it on stdin keeps the secret out of the process table, where a
	// command-line argument would be visible to every user on the box.
	login func() string
	// verify reports whether the tool is already authenticated.
	verify string
}

// SupportedTools is the set cbx-setuptool can authenticate. Claude Code is not
// here: its subscription login is an interactive browser OAuth with no token
// path, so it is handled separately.
func SupportedTools() []ToolAuth {
	return []ToolAuth{
		{
			Name:     "github",
			TokenURL: "https://github.com/settings/tokens",
			Help:     "lets sessions clone private repos and open pull requests",
			login:    func() string { return "gh auth login --with-token" },
			verify:   "gh auth status",
		},
		{
			Name:     "vercel",
			TokenURL: "https://vercel.com/account/tokens",
			Help:     "lets sessions deploy and inspect Vercel projects",
			// vercel has no stdin login; it reads VERCEL_TOKEN from the
			// environment or --token. Writing the config file directly is the
			// only way to persist it without putting the token in argv.
			// printf is a shell builtin, so the token never becomes the argv
			// of a forked process and never appears in the process table.
			// umask makes the file private from the moment it exists rather
			// than chmod-ing a briefly world-readable one.
			login: func() string {
				return `set -e; TOKEN=$(cat); ` +
					`mkdir -p "$HOME/.local/share/com.vercel.cli"; ` +
					`umask 077; ` +
					`printf '{"token":"%s"}\n' "$TOKEN" > "$HOME/.local/share/com.vercel.cli/auth.json"`
			},
			verify: "vercel whoami",
		},
		{
			Name:     "supabase",
			TokenURL: "https://supabase.com/dashboard/account/tokens",
			Help:     "lets sessions manage Supabase projects and edge functions",
			login:    func() string { return "supabase login --token \"$(cat)\"" },
			verify:   "supabase projects list",
		},
	}
}

// Authenticate sends a token to the target and runs the tool's login, then
// verifies it took.
//
// The token goes over stdin rather than in the command, so it never appears in
// the box's process table or in shell history. It is also never written to a
// file by cbx — whatever the tool does with it afterwards is the tool's
// business.
func Authenticate(t Target, tool ToolAuth, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("%s: empty token", tool.Name)
	}
	if strings.ContainsAny(token, "\n\r") {
		return fmt.Errorf("%s: token contains a newline", tool.Name)
	}

	cmd := exec_ssh(t, tool.login())
	cmd.Stdin = strings.NewReader(token)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s login on %s: %w: %s", tool.Name, t, err, strings.TrimSpace(string(out)))
	}
	if _, err := Run(t, tool.verify+" >/dev/null 2>&1"); err != nil {
		return fmt.Errorf("%s: the token was accepted but %q still fails — is the token valid and unexpired?", tool.Name, tool.verify)
	}
	return nil
}

// IsAuthenticated reports whether a tool is already logged in on the target,
// so the tool can skip asking for a token it does not need.
func IsAuthenticated(t Target, tool ToolAuth) bool {
	_, err := Run(t, tool.verify+" >/dev/null 2>&1")
	return err == nil
}

// TokenFromEnv returns a token for a tool from the local environment, so
// someone who already has GITHUB_TOKEN exported is not asked to paste it.
func TokenFromEnv(tool ToolAuth) string {
	for _, k := range envKeys[tool.Name] {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

var envKeys = map[string][]string{
	"github":   {"GH_TOKEN", "GITHUB_TOKEN"},
	"vercel":   {"VERCEL_TOKEN"},
	"supabase": {"SUPABASE_ACCESS_TOKEN"},
}

// ClaudeLoggedIn reports whether Claude Code is signed in on the box.
func ClaudeLoggedIn(t Target) bool {
	out, err := Run(t, toolPath+`claude auth status --json 2>/dev/null`)
	if err != nil {
		return false
	}
	return strings.Contains(out, `"loggedIn": true`) || strings.Contains(out, `"loggedIn":true`)
}

// ClaudeLogin hands the caller's terminal to Claude Code on the box so they can
// run /login themselves.
//
// There is no token path for a subscription account — the sign-in is a browser
// OAuth. Driving it by scraping the TUI was tried and deleted: it matched a
// dozen literal UI strings and broke silently whenever any of them changed.
func ClaudeLogin(t Target) error {
	return Interactive(t, toolPath+"claude")
}
