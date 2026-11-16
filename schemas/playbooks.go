package schemas

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"sync"
)

//go:embed playbooks.schema.json
var playbookSchemaJSON []byte

//go:embed playbooks.json
var playbookData []byte

var playbookSchema = cachedSchema("urn:forgeos:playbooks:v1", playbookSchemaJSON)

type Playbook struct {
	Title          string   `json:"title"`
	Objective      string   `json:"objective"`
	Preconditions  string   `json:"preconditions"`
	Steps          []string `json:"steps"`
	ExpectedResult string   `json:"expected_result"`
	Evidence       []string `json:"evidence"`
}

type PlaybookLibrary struct {
	Version   int                 `json:"version"`
	SHA256    string              `json:"sha256"`
	Playbooks map[string]Playbook `json:"playbooks"`
}

// GlobalPlaybooks returns the read-only catalog shipped with this build.
var GlobalPlaybooks = sync.OnceValues(func() (PlaybookLibrary, error) {
	var library PlaybookLibrary
	if err := decodeConfig(playbookData, playbookSchema, &library); err != nil {
		return library, err
	}
	library.SHA256 = fmt.Sprintf("%x", sha256.Sum256(playbookData))
	return library, nil
})
