package main

import (
	"strings"
	"testing"
)

func TestModelOutputRedactsWithoutCredentialProvider(t *testing.T) {
	output := modelOutput{secrets: []string{"preview-password-example"}}
	safe, err := output.redact("preview started\npreview-password-example")
	if err != nil || !strings.Contains(safe, "preview started") || strings.Contains(safe, "preview-password-example") {
		t.Fatalf("unexpected preview log output %q, error %v", safe, err)
	}
}
