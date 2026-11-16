package validation

import "time"

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
	Profile       string        `json:"profile,omitempty"`
	Image         string        `json:"image,omitempty"`
	Started       time.Time     `json:"started_at"`
	Finished      time.Time     `json:"finished_at"`
	Checks        []CheckResult `json:"checks"`
	Failure       string        `json:"failure,omitempty"`
	Detail        string        `json:"detail,omitempty"`
}

type CheckResult struct {
	Name     string    `json:"name"`
	Required bool      `json:"required"`
	Started  time.Time `json:"started_at"`
	Finished time.Time `json:"finished_at"`
	ExitCode *int      `json:"exit_code"`
	Status   string    `json:"status"`
	Failure  string    `json:"failure,omitempty"`
	Log      string    `json:"log"`
	Outputs  []Output  `json:"outputs,omitempty"`
}

type Output struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
