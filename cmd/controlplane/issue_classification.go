package main

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"forgeos/internal/store"
)

func issueSourceDigest(issue store.Issue) string {
	content := strings.Join([]string{issue.URL, issue.Title, issue.Body, issue.UpdatedAt, issue.WorkType, issue.WorkTypeLabels}, "\x00")
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

// An explicit GitHub label and a bracketed title prefix are independent declarations.
// Conflicting declarations require clarification, regardless of the order GitHub returns them.
func (i githubIssue) classify() (string, string) {
	kind := ""
	var labels []string
	add := func(value string) {
		if !store.ValidWorkType(value) || kind != "" && value != kind {
			kind = "clarification"
			return
		}
		kind = value
	}
	for _, label := range i.Labels {
		if value, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(label.Name)), "forgeos:type:"); ok {
			add(value)
			if !store.ValidWorkType(value) {
				value = "?"
			}
			labels = append(labels, value)
		}
	}
	title := strings.TrimSpace(i.Title)
	for strings.HasPrefix(title, "[") {
		prefix, rest, ok := strings.Cut(title[1:], "]")
		if !ok {
			break
		}
		value := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(prefix)), " ", "-")
		switch value {
		case "开发":
			value = "development"
		case "代码审查":
			value = "code-review"
		case "测试":
			value = "testing"
		case "调查":
			value = "investigation"
		case "运维":
			value = "operations"
		}
		if store.ValidWorkType(value) {
			add(value)
		}
		title = strings.TrimSpace(rest)
	}
	if kind == "" && i.hasAgentLabel() {
		parts := strings.Split(actionSeparators.Replace(strings.ToLower(title)), "\n")
		if first := titleAction(parts[0]); first != "" {
			add(first)
			for _, part := range parts[1:] {
				if next := titleAction(part); next != "" {
					add(next)
				}
			}
		}
	}
	if kind == "" && i.hasAgentLabel() {
		kind = "clarification"
	}
	slices.Sort(labels)
	return kind, strings.Join(slices.Compact(labels), ",")
}

var actionSeparators = strings.NewReplacer(", then ", "\n", " and then ", "\n", " and ", "\n", " then ", "\n", "; ", "\n", " & ", "\n")

func titleAction(text string) string {
	verb, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	switch verb {
	case "add", "implement", "fix", "update", "remove", "document", "refactor":
		return "development"
	case "review":
		return "code-review"
	case "test", "verify":
		return "testing"
	case "investigate", "diagnose":
		return "investigation"
	case "deploy", "restore", "restart":
		return "operations"
	}
	return ""
}
