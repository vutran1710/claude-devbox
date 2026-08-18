package provision

import (
	"encoding/json"
	"fmt"
)

// claudeConfig builds the claude user's ~/.claude.json, merging into whatever
// is already there rather than replacing it — that file is where Claude Code
// records the signed-in account, so overwriting it logs the user out.
//
// hasCompletedOnboarding is essential: without it Claude Code re-runs
// onboarding on every start and a session comes up on a theme picker and a
// login screen instead of its prompt, even when credentials are valid.
func claudeConfig(existing []byte) ([]byte, error) {
	cfg := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &cfg); err != nil {
			return nil, fmt.Errorf("parse existing claude config: %w", err)
		}
	}

	cfg["hasCompletedOnboarding"] = true

	return json.MarshalIndent(cfg, "", "  ")
}
