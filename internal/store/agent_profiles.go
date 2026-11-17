package store

type AgentProfile struct {
	Version    int      `json:"version"`
	Sandbox    string   `json:"sandbox,omitempty"`
	Permission string   `json:"permission"`
	Tools      []string `json:"tools,omitempty"`
}

const (
	CodexImplementationSandbox = "workspace-write"
	CodexPlannerSandbox        = "read-only"
	CodexApproval              = "never"
)

func ClaudeImplementationProfile() AgentProfile {
	return AgentProfile{Version: 1, Permission: "acceptEdits", Tools: []string{"Read", "Glob", "Grep", "Edit", "Write"}}
}

// Profiles reports only roles enabled on an ordinary Worker.
func (c WorkerCapabilities) Profiles(harness string) map[string]AgentProfile {
	profiles := map[string]AgentProfile{}
	switch {
	case harness == "codex" && c.SupportsCodex:
		profiles["implementation"] = AgentProfile{Version: 1, Sandbox: CodexImplementationSandbox, Permission: CodexApproval}
		if c.SupportsPlanner {
			profiles["planner"] = AgentProfile{Version: 1, Sandbox: CodexPlannerSandbox, Permission: CodexApproval}
		}
	case harness == "claude" && c.SupportsClaude:
		profiles["implementation"] = ClaudeImplementationProfile()
	}
	return profiles
}
