package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

func visibleMCPBlock(block map[string]any) bool {
	annotations, ok := block["annotations"].(map[string]any)
	if !ok {
		return block["annotations"] == nil
	}
	audience, present := annotations["audience"]
	if !present {
		return true
	}
	roles, _ := audience.([]any)
	for _, role := range roles {
		if role == "user" {
			return true
		}
	}
	return false
}

// Keep public text and JSON data. Transport metadata and embedded resources are not log content.
func publicMCPValue(value any, depth int) (any, bool) {
	if depth > 64 {
		return nil, false
	}
	switch v := value.(type) {
	case map[string]any:
		if !visibleMCPBlock(v) {
			return nil, false
		}
		switch v["type"] {
		case "image", "audio", "resource", "resource_link":
			return nil, false
		}
		for k, child := range v {
			switch strings.ToLower(k) {
			case "_meta", "annotations", "blob", "uri", "mimetype":
				delete(v, k)
			default:
				if _, opaque := child.(string); strings.EqualFold(k, "data") && opaque {
					delete(v, k)
					continue
				}
				if clean, ok := publicMCPValue(child, depth+1); ok {
					v[k] = clean
				} else {
					delete(v, k)
				}
			}
		}
	case []any:
		out := v[:0]
		for _, child := range v {
			if clean, ok := publicMCPValue(child, depth+1); ok {
				out = append(out, clean)
			}
		}
		return out, true
	}
	return value, true
}

func mcpToolOutput(result, failure json.RawMessage) string {
	var response struct {
		Content       []any `json:"content"`
		Structured    any   `json:"structuredContent"`
		CLIStructured any   `json:"structured_content"`
	}
	var parts []string
	d := json.NewDecoder(bytes.NewReader(result))
	d.UseNumber()
	if d.Decode(&response) == nil {
		for _, block := range response.Content {
			item, _ := block.(map[string]any)
			if text, ok := item["text"].(string); ok && text != "" && item["type"] == "text" && visibleMCPBlock(item) {
				parts = append(parts, text)
			}
		}
		if response.Structured == nil {
			response.Structured = response.CLIStructured
		}
		if response.Structured != nil {
			if clean, ok := publicMCPValue(response.Structured, 0); ok {
				var out bytes.Buffer
				enc := json.NewEncoder(&out)
				enc.SetEscapeHTML(false)
				if enc.Encode(clean) == nil {
					parts = append(parts, strings.TrimSuffix(out.String(), "\n"))
				}
			}
		}
	}
	var err struct{ Message string }
	if json.Unmarshal(failure, &err) == nil && err.Message != "" {
		parts = append(parts, err.Message)
	}
	return strings.Join(parts, "\n")
}
