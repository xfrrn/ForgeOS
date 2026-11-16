package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const LogLimit = 1 << 20
const LogTruncation = "\n[remaining CI output omitted]\n"
const MaxLogBytes = LogLimit + len(LogTruncation)

// Run identifies the immutable inputs and the outputs retained by the runner.
type Run struct {
	ExecutionID   int64         `json:"execution_id,omitempty"`
	ID            string        `json:"id"`
	Runner        string        `json:"runner"`
	Base          string        `json:"base_commit"`
	Commit        string        `json:"commit"`
	ConfigBlob    string        `json:"config_blob,omitempty"`
	ConfigVersion int           `json:"config_version,omitempty"`
	Level         string        `json:"level"`
	Decision      *Decision     `json:"decision,omitempty"`
	Profile       string        `json:"profile,omitempty"`
	Image         string        `json:"image,omitempty"`
	Started       time.Time     `json:"started_at"`
	Finished      time.Time     `json:"finished_at"`
	Checks        []CheckResult `json:"checks"`
	Failure       string        `json:"failure,omitempty"`
	Detail        string        `json:"detail,omitempty"`
}

type CheckResult struct {
	Name      string      `json:"name"`
	Required  bool        `json:"required"`
	Started   time.Time   `json:"started_at"`
	Finished  time.Time   `json:"finished_at"`
	ExitCode  *int        `json:"exit_code"`
	Status    string      `json:"status"`
	Failure   string      `json:"failure,omitempty"`
	Log       string      `json:"log"`
	LogDigest *FileDigest `json:"log_digest,omitempty"`
	Outputs   []Output    `json:"outputs,omitempty"`
}

type FileDigest struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func DigestBytes(data []byte) FileDigest {
	sum := sha256.Sum256(data)
	return FileDigest{Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
}

func (r *Run) Files() map[string]FileDigest {
	files := make(map[string]FileDigest)
	for _, c := range r.Checks {
		if c.LogDigest != nil {
			files[c.Log] = *c.LogDigest
		}
		for _, o := range c.Outputs {
			files[o.File] = FileDigest{Bytes: o.Bytes, SHA256: o.SHA256}
		}
	}
	return files
}

type Output struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
