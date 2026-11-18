package store

import (
	"reflect"

	"forgeos/schemas"
)

type AgentProfile = schemas.AgentProfile

const (
	CodexImplementationSandbox = "workspace-write"
	CodexPlannerSandbox        = "read-only"
	CodexApproval              = "never"
)

func ClaudeImplementationProfile() AgentProfile {
	return AgentProfile{Version: 1, Permission: "acceptEdits", Tools: &[]string{"Read", "Glob", "Grep", "Edit", "Write"}}
}

func CodexDisabledFeatures() []string {
	return []string{"apps", "plugins", "hooks"}
}

func StructuredCodexProfile() AgentProfile {
	return AgentProfile{Version: 1, Sandbox: CodexPlannerSandbox, Permission: CodexApproval,
		DisabledFeatures: append(CodexDisabledFeatures(), "shell_tool", "unified_exec", "multi_agent", "multi_agent_v2",
			"code_mode", "code_mode_host", "computer_use", "browser_use", "view_image", "image_generation")}
}

func validStructuredProfile(p *AgentProfile) bool {
	return p == nil || reflect.DeepEqual(*p, StructuredCodexProfile())
}

// Profiles reports only roles enabled on an ordinary Worker.
func (c WorkerCapabilities) Profiles(harness string) map[string]AgentProfile {
	profiles := map[string]AgentProfile{}
	switch {
	case harness == "codex" && c.SupportsCodex:
		profiles["implementation"] = AgentProfile{Version: 1, Sandbox: CodexImplementationSandbox, Permission: CodexApproval}
		if c.CommandApprovalProject > 0 {
			profiles["implementation_with_approval"] = AgentProfile{Version: 1, Sandbox: CodexImplementationSandbox, Permission: "untrusted"}
		}
		if c.SupportsPlanner {
			profiles["planner"] = AgentProfile{Version: 1, Sandbox: CodexPlannerSandbox, Permission: CodexApproval}
		}
	case harness == "claude" && c.SupportsClaude:
		profiles["implementation"] = ClaudeImplementationProfile()
	}
	return profiles
}
