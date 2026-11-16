package schemas

import (
	"os"
	"testing"
)

func TestGlobalPlaybooksCoverRoadmapAndExample(t *testing.T) {
	library, err := GlobalPlaybooks()
	if err != nil || library.Version != 1 || len(library.SHA256) != 64 {
		t.Fatal(library.Version, library.SHA256, err)
	}
	for _, id := range []string{"frontend-ui", "api-backend", "database-migration", "auth-authorization", "payment", "cloud-integration", "async-jobs", "ai-media", "desktop", "mobile", "deployment", "performance"} {
		if _, found := library.Playbooks[id]; !found {
			t.Errorf("missing roadmap playbook %s", id)
		}
	}
	raw, err := os.ReadFile("acceptance.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseAcceptance(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cfg.Components {
		for _, id := range c.Playbooks {
			if _, found := library.Playbooks[id]; !found {
				t.Errorf("unresolved example playbook %s", id)
			}
		}
	}
}
