package setuptool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Dropped records something migrate refused to send, and why. Callers print
// these: silently shipping a broken hook is worse than shipping less, because
// it fails on every edit inside a session with no obvious cause.
type Dropped struct {
	Path   string
	Reason string
}

// PortableSettings rewrites a settings.json so it works on the target.
//
// Copied verbatim, a real settings.json breaks the box. A hook like
//
//	cat >/dev/null || true; git rev-parse --git-dir >/dev/null 2>&1 && code-review-graph update --repo "/Users/you"
//
// references a home directory that does not exist there and a binary that is
// not installed, and fires on every single edit.
func PortableSettings(raw []byte, localHome, remoteHome string, hasBinary func(string) bool) ([]byte, []Dropped, error) {
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse settings.json: %w", err)
	}
	var dropped []Dropped

	if sl, ok := cfg["statusLine"].(map[string]any); ok {
		cmd, _ := sl["command"].(string)
		if missing := missingBinary(cmd, hasBinary); missing != "" {
			delete(cfg, "statusLine")
			dropped = append(dropped, Dropped{"statusLine", fmt.Sprintf("%q not on the target", missing)})
		} else {
			sl["command"] = rewriteHome(cmd, localHome, remoteHome)
		}
	}

	if hooks, ok := cfg["hooks"].(map[string]any); ok {
		for event, v := range hooks {
			matchers, ok := v.([]any)
			if !ok {
				continue
			}
			kept := make([]any, 0, len(matchers))
			for _, m := range matchers {
				entry, ok := m.(map[string]any)
				if !ok {
					continue
				}
				inner, _ := entry["hooks"].([]any)
				keptInner := make([]any, 0, len(inner))
				for _, h := range inner {
					hook, ok := h.(map[string]any)
					if !ok {
						continue
					}
					cmd, _ := hook["command"].(string)
					if missing := missingBinary(cmd, hasBinary); missing != "" {
						dropped = append(dropped, Dropped{"hooks." + event, fmt.Sprintf("%q not on the target", missing)})
						continue
					}
					hook["command"] = rewriteHome(cmd, localHome, remoteHome)
					keptInner = append(keptInner, hook)
				}
				if len(keptInner) == 0 {
					continue
				}
				entry["hooks"] = keptInner
				kept = append(kept, entry)
			}
			if len(kept) == 0 {
				delete(hooks, event)
				continue
			}
			hooks[event] = kept
		}
		if len(hooks) == 0 {
			delete(cfg, "hooks")
		}
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	return out, dropped, err
}

// missingBinary returns the first binary in a command that the target lacks.
//
// A shell command is a pipeline of segments, and a real hook is compound:
// `cat >/dev/null || true; git ... && code-review-graph ...`. An earlier
// version gave up on the first redirection and returned nothing, so every
// compound hook was kept regardless of what it invoked.
func missingBinary(command string, hasBinary func(string) bool) string {
	for _, seg := range splitSegments(command) {
		bin := segmentBinary(seg)
		if bin == "" || isBuiltin(bin) {
			continue
		}
		if !hasBinary(bin) {
			return bin
		}
	}
	return ""
}

func splitSegments(cmd string) []string {
	r := strings.NewReplacer("&&", "\x00", "||", "\x00", ";", "\x00", "|", "\x00", "\n", "\x00")
	return strings.Split(r.Replace(cmd), "\x00")
}

// segmentBinary finds what a segment invokes, stepping over env assignments,
// `env`, and redirections.
func segmentBinary(seg string) string {
	for _, f := range strings.Fields(seg) {
		switch {
		case f == "env", f == "sudo", f == "exec", f == "command":
			continue
		case strings.HasPrefix(f, "-"): // a flag on env/sudo
			continue
		case strings.ContainsRune(f, '='): // FOO=bar
			continue
		case strings.HasPrefix(f, ">"), strings.HasPrefix(f, "<"),
			strings.HasPrefix(f, "2>"), strings.HasPrefix(f, "&"):
			continue
		case strings.HasPrefix(f, "$"): // unresolvable, assume present
			return ""
		}
		return strings.Trim(f, `"'`)
	}
	return ""
}

// isBuiltin covers shell builtins and keywords, which are never absent and
// must not cause a hook to be dropped.
func isBuiltin(s string) bool {
	switch s {
	case "true", "false", "echo", "cd", "test", "[", "printf", "set", "export",
		"cat", "read", "eval", "source", ".", "if", "then", "else", "fi", "sh", "bash", "zsh":
		return true
	}
	return false
}

// rewriteHome replaces the local home prefix with the target's.
//
// Both boundaries matter. Without a trailing check, "/Users/vu" mangles
// "/Users/vutran/..."; without a leading one, "/home/pi" corrupts the
// unrelated "/mnt/home/pi/backup". The trailing boundary is any character that
// cannot continue a path segment — a quote counts, which is how
// `--repo "/Users/you"` gets rewritten at all.
func rewriteHome(s, localHome, remoteHome string) string {
	if localHome == "" || localHome == "/" {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], localHome) &&
			isBoundary(s, i-1) && isBoundary(s, i+len(localHome)) {
			b.WriteString(remoteHome)
			i += len(localHome)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// isBoundary reports whether the byte at i cannot be part of a path segment.
// Out of range counts, so start and end of string are boundaries. '/' counts,
// so a match followed by a subpath still rewrites.
func isBoundary(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	if c == '/' {
		return true
	}
	return !(c == '.' || c == '-' || c == '_' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
}
