package schemas

import (
	"os"
	"strings"
	"testing"
)

func TestValidationSchema(t *testing.T) {
	raw, err := os.ReadFile("validation.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	config, err := ParseValidation(raw)
	if err != nil || len(config.Checks) != 6 {
		t.Fatalf("parse example: %v", err)
	}
	for name, change := range map[string][2]string{
		"unknown privilege":       {"type: static", "privileged: true\n    type: static"},
		"escaping cwd":            {"type: static", "working_directory: ../outside\n    type: static"},
		"escaping artifact":       {"type: static", "artifacts: [../outside]\n    type: static"},
		"absolute win path":       {"type: static", "reports: ['C:/outside']\n    type: static"},
		"duplicate key":           {"type: static", "type: unit\n    type: static"},
		"unsupported environment": {"environment: ci", "environment: production"},
		"missing check":           {"checks: [format, lint, typecheck]", "checks: [missing]"},
		"wrong argument type":     {`"pnpm", "format:check"`, `"pnpm", 30`},
		"multiple documents":      {"version: 1", "version: 1\n---\nversion: 1"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseValidation([]byte(strings.Replace(string(raw), change[0], change[1], 1)))
			if err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
