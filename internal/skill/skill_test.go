package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestDiscoverAndLoadSkillOnDemand(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "review", `{"version":1,"name":"review","skill_version":"1.0.0","description":"Review code","allowed_tools":["read_file"],"workspace_scopes":["."],"locked":true}`, "# Review\nInspect evidence first.")
	definitions := []contracts.ToolDefinition{{Name: "read_file", RiskClass: contracts.RiskRead}}
	items, err := Discover(root, definitions)
	if err != nil || len(items) != 1 || items[0].Name != "review" || items[0].Locked != true {
		t.Fatal(items, err)
	}
	loaded, err := Load(root, "review", definitions)
	if err != nil || loaded.Instructions != "# Review\nInspect evidence first." {
		t.Fatal(loaded, err)
	}
}

func TestDiscoverRejectsUnknownTool(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "writer", `{"version":1,"name":"writer","skill_version":"1.0.0","description":"Write","allowed_tools":["write_file"],"workspace_scopes":["."]}`, "# Write")
	_, err := Discover(root, []contracts.ToolDefinition{{Name: "read_file", RiskClass: contracts.RiskRead}})
	if domain, ok := err.(*contracts.Error); !ok || domain.Code != contracts.ErrToolNotAllowed {
		t.Fatalf("expected tool deny, got %#v", err)
	}
}

func writeSkill(t *testing.T, root, name, manifest, instructions string) {
	t.Helper()
	directory := filepath.Join(root, ".simpleness", "skills", name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, instructionsName), []byte(instructions), 0o644); err != nil {
		t.Fatal(err)
	}
}
