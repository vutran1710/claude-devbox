package setuptool

import (
	"strings"
	"testing"
)

// The fixture below is the shape of a real settings.json, not a simplified
// one. Earlier versions of this code were tested against `tool --repo "/x"`
// and passed, then shipped hooks that fired on every edit on the box, because
// the real thing is compound and quote-terminated.
const realWorldSettings = `{
  "hooks": {
    "PostToolUse": [{"matcher":"Edit|Write|Bash","hooks":[{"type":"command",
      "command":"cat >/dev/null || true; git rev-parse --git-dir >/dev/null 2>&1 && code-review-graph update --skip-flows --repo \"/Users/vutran\" || true"}]}],
    "SessionStart": [{"matcher":"","hooks":[{"type":"command",
      "command":"cat >/dev/null || true; git rev-parse --git-dir >/dev/null 2>&1 && code-review-graph status --repo \"/Users/vutran\" || echo 'Not a git repo'"}]}]
  },
  "statusLine": {"type":"command","command":"sh /Users/vutran/.claude/statusline-command.sh"},
  "theme": "dark",
  "enabledPlugins": {"vercel@official": true}
}`

func present(string) bool { return true }
func absent(bin string) bool {
	return bin != "code-review-graph" && bin != "/Users/vutran/.claude/statusline-command.sh"
}

func TestCompoundHookIsDroppedWhenItsBinaryIsMissing(t *testing.T) {
	out, dropped, err := PortableSettings([]byte(realWorldSettings), "/Users/vutran", "/root", absent)
	if err != nil {
		t.Fatalf("PortableSettings: %v", err)
	}
	if strings.Contains(string(out), "code-review-graph") {
		t.Errorf("a hook invoking a missing binary survived:\n%s", out)
	}
	if len(dropped) < 2 {
		t.Errorf("dropped = %v, want both hooks reported", dropped)
	}
}

// The whole point: nothing on the box may still reference the operator's home.
func TestNoLocalPathsSurvive(t *testing.T) {
	out, _, err := PortableSettings([]byte(realWorldSettings), "/Users/vutran", "/root", present)
	if err != nil {
		t.Fatalf("PortableSettings: %v", err)
	}
	if strings.Contains(string(out), "/Users/vutran") {
		t.Errorf("a local path survived rewriting:\n%s", out)
	}
	if !strings.Contains(string(out), "/root") {
		t.Errorf("nothing was rewritten to the target home:\n%s", out)
	}
}

// The path ends at a quote, not a slash. A trailing-boundary check that only
// accepts '/' or end-of-string misses this entirely.
func TestQuoteTerminatedPathIsRewritten(t *testing.T) {
	in := `{"hooks":{"E":[{"matcher":"","hooks":[{"type":"command","command":"tool --repo \"/Users/vutran\""}]}]}}`
	out, _, _ := PortableSettings([]byte(in), "/Users/vutran", "/root", present)
	if !strings.Contains(string(out), `--repo \"/root\"`) && !strings.Contains(string(out), `--repo \"/root`) {
		t.Errorf("quote-terminated path not rewritten:\n%s", out)
	}
}

func TestShortPrefixDoesNotCorruptLongerPaths(t *testing.T) {
	for _, tc := range []struct{ local, in, banned string }{
		{"/Users/vu", `{"statusLine":{"command":"sh /Users/vutran/x.sh"}}`, "/roottran"},
		{"/home/pi", `{"statusLine":{"command":"sh /mnt/home/pi/backup/x.sh"}}`, "/mnt/root/backup"},
	} {
		out, _, _ := PortableSettings([]byte(tc.in), tc.local, "/root", present)
		if strings.Contains(string(out), tc.banned) {
			t.Errorf("localHome %q corrupted an unrelated path (%q):\n%s", tc.local, tc.banned, out)
		}
	}
}

func TestBuiltinsDoNotCauseADrop(t *testing.T) {
	in := `{"hooks":{"E":[{"matcher":"","hooks":[{"type":"command","command":"echo hi; true"}]}]}}`
	out, dropped, _ := PortableSettings([]byte(in), "/Users/x", "/root", func(string) bool { return false })
	if len(dropped) != 0 {
		t.Errorf("dropped a hook that only uses builtins: %v", dropped)
	}
	if !strings.Contains(string(out), "echo hi") {
		t.Errorf("the hook was removed:\n%s", out)
	}
}

func TestUnrelatedKeysSurvive(t *testing.T) {
	out, _, _ := PortableSettings([]byte(realWorldSettings), "/Users/vutran", "/root", absent)
	for _, want := range []string{`"theme"`, `"dark"`, `"enabledPlugins"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("%s was lost:\n%s", want, out)
		}
	}
}

func TestMalformedSettingsIsAnErrorNotASilentPass(t *testing.T) {
	if _, _, err := PortableSettings([]byte("{not json"), "/a", "/b", present); err == nil {
		t.Error("expected an error rather than shipping unreadable settings")
	}
}
