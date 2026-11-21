package chatkernel

import (
	"net"
	"net/url"
	"strings"
	"unicode"
)

// Provider is transmitted only on the project-authorized, lease-checked credential endpoint.
type Provider struct {
	BaseURL      string `json:"base_url"`
	Model        string `json:"model"`
	APIKey       string `json:"api_key"`
	OutputFormat string `json:"output_format,omitempty"`
}

func (p Provider) Valid() bool {
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(p.BaseURL) > 2048 {
		return false
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())) {
		return false
	}
	return modelName.MatchString(p.Model) && len(p.APIKey) > 0 && len(p.APIKey) <= 4096 &&
		strings.IndexFunc(p.APIKey, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0 &&
		(p.OutputFormat == "" || p.OutputFormat == "json_object" || p.OutputFormat == "json_schema")
}
