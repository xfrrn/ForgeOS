package store

import (
	"reflect"

	"forgeos/internal/review"
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

func CodexDisabledFeatures() []string      { return schemas.CodexDisabledFeatures() }
func StructuredCodexProfile() AgentProfile { return schemas.StructuredCodexProfile() }
func ClaudePlannerProfile() AgentProfile {
	return AgentProfile{Version: 1, Permission: "dontAsk", Tools: &[]string{"Read", "Glob", "Grep"}}
}

func validStructuredProfile(p *AgentProfile) bool {
	return p == nil || reflect.DeepEqual(*p, StructuredCodexProfile()) || reflect.DeepEqual(*p, review.ClaudeProfile())
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
		if c.ClaudePlanning {
			profiles["planner"] = ClaudePlannerProfile()
		}
	}
	return profiles
}
