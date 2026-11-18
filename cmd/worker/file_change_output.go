package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type nativeFileChange struct {
	Path string
	Kind json.RawMessage
	Diff string
}

func fileChangeOutput(changes []nativeFileChange) string {
	var output strings.Builder
	for _, change := range changes {
		var kind string
		var detail struct {
			Type     string
			MovePath string `json:"move_path"`
		}
		if json.Unmarshal(change.Kind, &kind) != nil {
			if json.Unmarshal(change.Kind, &detail) != nil {
				continue
			}
			kind = detail.Type
		}
		if change.Path == "" || (kind != "add" && kind != "delete" && kind != "update") {
			continue
		}
		fmt.Fprintf(&output, "%s %q", kind, change.Path)
		if kind == "update" && detail.MovePath != "" {
			fmt.Fprintf(&output, " -> %q", detail.MovePath)
		}
		output.WriteByte('\n')
		if change.Diff != "" {
			output.WriteString(change.Diff)
			if !strings.HasSuffix(change.Diff, "\n") {
				output.WriteByte('\n')
			}
		}
	}
	return output.String()
}
