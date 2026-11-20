package schemas

// A nil Tools field leaves tool selection unspecified. An explicit empty list disables the harness's built-in tools.
type AgentProfile struct {
	Version          int       `json:"version"`
	Sandbox          string    `json:"sandbox,omitempty"`
	Permission       string    `json:"permission"`
	Tools            *[]string `json:"tools,omitempty"`
	DisabledFeatures []string  `json:"disabled_features,omitempty"`
}

func CodexDisabledFeatures() []string {
	return []string{"apps", "plugins", "hooks"}
}

func StructuredCodexProfile() AgentProfile {
	return AgentProfile{Version: 1, Sandbox: "read-only", Permission: "never",
		DisabledFeatures: append(CodexDisabledFeatures(), "shell_tool", "unified_exec", "multi_agent", "multi_agent_v2",
			"code_mode", "code_mode_host", "computer_use", "browser_use", "view_image", "image_generation")}
}
