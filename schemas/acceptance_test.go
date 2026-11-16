package schemas

import (
	"os"
	"strings"
	"testing"
)

func TestAcceptanceExampleAndGlob(t *testing.T) {
	raw, err := os.ReadFile("acceptance.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseAcceptance(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Components["payment"]
	if c.Minimum != "V4" || !c.Matches("internal/payment/api/charge.go") || c.Matches("internal/payment-old/charge.go") || c.Manual.Environment != "staging" {
		t.Fatal(c)
	}
	cfg, err = ParseAcceptance([]byte(strings.Replace(string(raw), "internal/payment/**", "internal/**/test?.go", 1)))
	if err != nil {
		t.Fatal(err)
	}
	c = cfg.Components["payment"]
	if !c.Matches("internal/test1.go") || !c.Matches("internal/one/two/testA.go") || c.Matches("internal/test12.go") {
		t.Fatal("incorrect segment match")
	}
	for _, pattern := range []string{"../outside", "internal/ab**cd", "/root", "internal\\bad"} {
		if _, err := ParseAcceptance([]byte(strings.Replace(string(raw), "internal/payment/**", pattern, 1))); err == nil {
			t.Fatalf("accepted %q", pattern)
		}
	}
}
