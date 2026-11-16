package schemas

import _ "embed"

//go:embed project.schema.json
var projectJSON []byte
var projectSchema = cachedSchema("urn:forgeos:project:v1", projectJSON)

func ProjectHarness(raw []byte) (string, error) {
	var project struct {
		Execution struct {
			DefaultHarness string `json:"default_harness"`
		} `json:"execution"`
	}
	err := decodeConfig(raw, projectSchema, &project)
	return project.Execution.DefaultHarness, err
}
