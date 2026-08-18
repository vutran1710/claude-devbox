package provision

import (
	"encoding/json"
	"testing"
)

// Without this marker Claude Code re-runs onboarding on every start, which
// hijacks the session with a theme picker and a login screen even though the
// user is already authenticated.
func TestClaudeConfig_MarksOnboardingComplete(t *testing.T) {
	out, err := claudeConfig(nil)
	if err != nil {
		t.Fatalf("claudeConfig: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["hasCompletedOnboarding"] != true {
		t.Errorf("hasCompletedOnboarding = %v, want true — onboarding will re-run and hijack sessions", got["hasCompletedOnboarding"])
	}
}

// The old code wrote this file wholesale, so re-running setup discarded the
// account Claude Code had recorded there.
func TestClaudeConfig_PreservesExistingKeys(t *testing.T) {
	existing := []byte(`{"oauthAccount":{"emailAddress":"dev@caissa.vn"},"userID":"abc123"}`)

	out, err := claudeConfig(existing)
	if err != nil {
		t.Fatalf("claudeConfig: %v", err)
	}

	var got map[string]any
	json.Unmarshal(out, &got)
	if _, ok := got["oauthAccount"]; !ok {
		t.Error("oauthAccount was discarded — the user is logged out by re-running setup")
	}
	if got["userID"] != "abc123" {
		t.Errorf("userID = %v, want abc123", got["userID"])
	}
	if got["hasCompletedOnboarding"] != true {
		t.Error("hasCompletedOnboarding not set when merging into an existing config")
	}
}

func TestClaudeConfig_RejectsMalformedExisting(t *testing.T) {
	if _, err := claudeConfig([]byte("{not json")); err == nil {
		t.Error("expected an error rather than silently discarding an unreadable config")
	}
}
