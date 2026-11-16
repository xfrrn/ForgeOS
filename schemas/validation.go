// Package schemas reads the versioned configuration declared by project repositories.
package schemas

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

//go:embed validation.schema.json
var schemaJSON []byte

var validationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	const uri = "urn:forgeos:validation:v1"
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err = c.AddResource(uri, doc); err != nil {
		return nil, err
	}
	return c.Compile(uri)
})

type Check struct {
	Type             string   `json:"type"`
	Command          []string `json:"command"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	Environment      string   `json:"environment,omitempty"`
	Required         *bool    `json:"required,omitempty"`
	Reports          []string `json:"reports,omitempty"`
	Artifacts        []string `json:"artifacts,omitempty"`
}

type Validation struct {
	Version  int              `json:"version"`
	Checks   map[string]Check `json:"checks"`
	Profiles map[string]struct {
		Checks []string `json:"checks"`
	} `json:"profiles"`
}

const MaxValidationBytes = 128 << 10

func ParseValidation(raw []byte) (Validation, error) {
	var config Validation
	if len(raw) > MaxValidationBytes {
		return config, errors.New("validation.yaml exceeds 128 KiB")
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := dec.Decode(&value); err != nil || dec.Decode(new(any)) != io.EOF {
		return config, errors.New("expected one YAML document with unique keys")
	}
	doc, err := json.Marshal(value)
	if err != nil || len(doc) > MaxValidationBytes*4 {
		return config, errors.New("invalid or oversized expanded YAML document")
	}
	schema, err := validationSchema()
	if err != nil {
		return config, err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return config, err
	}
	if err = schema.Validate(instance); err != nil {
		return config, err
	}
	if err = json.Unmarshal(doc, &config); err != nil {
		return config, err
	}
	for name, c := range config.Checks {
		if c.WorkingDirectory != "" && !fs.ValidPath(c.WorkingDirectory) {
			return config, fmt.Errorf("invalid working directory for check %s", name)
		}
		for _, paths := range [][]string{c.Reports, c.Artifacts} {
			for _, p := range paths {
				if p == "." || !fs.ValidPath(p) {
					return config, fmt.Errorf("invalid output path for check %s", name)
				}
			}
		}
	}
	for name, p := range config.Profiles {
		for _, id := range p.Checks {
			if _, ok := config.Checks[id]; !ok {
				return config, fmt.Errorf("profile %s refers to missing check %s", name, id)
			}
		}
	}
	return config, nil
}
