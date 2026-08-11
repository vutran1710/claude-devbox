package provision

import (
	"encoding/json"
	"fmt"
)

// chromeMCPServer is the Chrome Lite MCP entry for the claude user's config.
var chromeMCPServer = map[string]any{
	"command": "node",
	"args":    []string{"/opt/chrome-lite-mcp/server/index.js"},
	"env":     map[string]any{},
}

// claudeConfig builds the claude user's ~/.claude.json, merging into whatever
// is already there rather than replacing it — that file is where Claude Code
// records the signed-in account, so overwriting it logs the user out.
//
// hasCompletedOnboarding is essential: without it Claude Code re-runs
// onboarding on every start and a session comes up on a theme picker and a
// login screen instead of its prompt, even when credentials are valid.
func claudeConfig(existing []byte, mcpEnabled bool) ([]byte, error) {
	cfg := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &cfg); err != nil {
			return nil, fmt.Errorf("parse existing claude config: %w", err)
		}
	}

	cfg["hasCompletedOnboarding"] = true
	if mcpEnabled {
		servers, _ := cfg["mcpServers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		servers["chrome"] = chromeMCPServer
		cfg["mcpServers"] = servers
	}

	return json.MarshalIndent(cfg, "", "  ")
}
