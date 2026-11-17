package review

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"forgeos/schemas"
)

type Finding struct {
	ID               string  `json:"id"`
	Severity         string  `json:"severity"`
	File             string  `json:"file"`
	Side             string  `json:"side"`
	Line             int     `json:"line"`
	Description      string  `json:"description"`
	Evidence         string  `json:"evidence"`
	Recommendation   string  `json:"recommendation"`
	Confidence       float64 `json:"confidence"`
	Blocking         bool    `json:"blocking"`
	EvidenceVerified bool    `json:"evidence_verified"`
}

type Report struct {
	Profile  *schemas.AgentProfile `json:"profile,omitempty"`
	Base     string                `json:"base_commit"`
	Commit   string                `json:"commit"`
	Session  string                `json:"session_id"`
	Summary  string                `json:"summary"`
	Findings []Finding             `json:"findings"`
}

func ClaudeProfile() schemas.AgentProfile {
	return schemas.AgentProfile{Version: 1, Permission: "dontAsk", Tools: &[]string{}}
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func (s Snapshot) Verify(r *Report) error {
	if r.Profile != nil && !reflect.DeepEqual(*r.Profile, ClaudeProfile()) {
		return errors.New("invalid Claude reviewer profile")
	}
	if r.Base != s.Base || r.Commit != s.Commit || !identifier.MatchString(r.Session) || strings.TrimSpace(r.Summary) == "" || len(r.Summary) > 2048 || r.Findings == nil || len(r.Findings) > 32 {
		return errors.New("invalid review identity, summary or findings")
	}
	ids := map[string]bool{}
	for i := range r.Findings {
		f := &r.Findings[i]
		if !identifier.MatchString(f.ID) || ids[f.ID] || f.File == "" || len(f.File) > 4096 || (f.Side != "base" && f.Side != "candidate") ||
			f.Line < 1 || strings.TrimSpace(f.Evidence) == "" || len(f.Evidence) > 4096 || strings.TrimSpace(f.Description) == "" || len(f.Description) > 2048 ||
			strings.TrimSpace(f.Recommendation) == "" || len(f.Recommendation) > 2048 || f.Confidence < 0 || f.Confidence > 1 {
			return errors.New("invalid review finding")
		}
		switch f.Severity {
		case "Critical", "High", "Medium", "Low":
		default:
			return errors.New("invalid review severity")
		}
		ids[f.ID] = true
		f.EvidenceVerified = false
		for _, file := range s.Files {
			if file.Path != f.File {
				continue
			}
			lines := file.After
			if f.Side == "base" {
				lines = file.Before
			}
			f.EvidenceVerified = f.Line <= len(lines) && strings.TrimSpace(lines[f.Line-1]) == strings.TrimSpace(f.Evidence)
			break
		}
	}
	return nil
}
