package schemas

// A nil Tools field leaves tool selection unspecified. An explicit empty list disables the harness's built-in tools.
type AgentProfile struct {
	Version          int       `json:"version"`
	Sandbox          string    `json:"sandbox,omitempty"`
	Permission       string    `json:"permission"`
	Tools            *[]string `json:"tools,omitempty"`
	DisabledFeatures []string  `json:"disabled_features,omitempty"`
}
