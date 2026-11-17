package validation

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

var runID = regexp.MustCompile("^ci-run-[0-9]{1,20}$")
var digest = regexp.MustCompile("^[0-9a-f]{64}$")
var checkName = regexp.MustCompile("^[a-z][a-z0-9_-]{0,63}$")

func ValidCheckName(name string) bool { return checkName.MatchString(name) }

func ValidRunID(id string) bool { return runID.MatchString(id) }

func (r *Run) Valid() bool {
	if !ValidRunID(r.ID) || r.Started.IsZero() || r.Finished.Before(r.Started) || r.Checks == nil || len(r.Checks) > 64 || len(r.Detail) > 2048 ||
		(r.Failure != "" && !knownFailure(r.Failure)) || (r.Image != "" && (len(r.Image) != 71 || !strings.HasPrefix(r.Image, "sha256:") || !digest.MatchString(r.Image[7:]))) {
		return false
	}
	prev := r.Started
	seen := map[string]bool{}
	var total int64
	for _, c := range r.Checks {
		if !ValidCheckName(c.Name) || seen[c.Name] || c.Log != c.Name+".log" || c.Started.Before(prev) || c.Finished.Before(c.Started) || c.Finished.After(r.Finished) || len(c.Outputs) > 64 {
			return false
		}
		seen[c.Name] = true
		if d := c.LogDigest; d != nil && (d.Bytes < 0 || d.Bytes > int64(MaxLogBytes) || !digest.MatchString(d.SHA256)) {
			return false
		}
		prev = c.Finished
		switch c.Status {
		case "PASSED":
			if c.Failure != "" || c.ExitCode == nil || *c.ExitCode != 0 {
				return false
			}
		case "FAILED":
			if !knownFailure(c.Failure) || (c.Required && r.Failure == "") {
				return false
			}
		default:
			return false
		}
		for j, o := range c.Outputs {
			if (o.Kind != "report" && o.Kind != "artifact") || o.Source == "." || !fs.ValidPath(o.Source) || len(o.Source) > 1024 ||
				!digest.MatchString(o.SHA256) || o.Bytes < 0 || o.Bytes > 8<<20 || o.File != fmt.Sprintf("%s-%d.bin", c.Name, j+1) {
				return false
			}
			total += o.Bytes
		}
	}
	return total <= 64<<20
}

// Accepts checks a passing receipt against the complete plan read by the control plane.
func (p Plan) Accepts(r *Run) bool {
	if !p.Ready || r == nil || !r.Valid() || r.Failure != "" || r.Image == "" ||
		r.Base != p.Commit || r.Level != p.Level || r.ConfigBlob != p.ConfigBlob || r.ConfigVersion != p.Version || r.Profile != p.Profile || len(r.Checks) != len(p.Checks) {
		return false
	}
	for i, c := range p.Checks {
		got := r.Checks[i]
		required := c.Required == nil || *c.Required
		if got.Name != c.Name || got.Required != required {
			return false
		}
		declared := map[[2]string]bool{}
		for _, v := range c.Reports {
			declared[[2]string{"report", v}] = true
		}
		for _, v := range c.Artifacts {
			declared[[2]string{"artifact", v}] = true
		}
		for _, o := range got.Outputs {
			key := [2]string{o.Kind, o.Source}
			if !declared[key] {
				return false
			}
			delete(declared, key)
		}
		if got.Status == "PASSED" && len(declared) > 0 {
			return false
		}
	}
	return true
}

func knownFailure(code string) bool {
	switch code {
	case "CODE_FAILURE", "BUILD_FAILURE", "TEST_ENV_FAILURE", "INFRA_FAILURE", "TIMEOUT", "CANCELLED", "CONFIG_FAILURE", "SECURITY_FAILURE":
		return true
	}
	return false
}
