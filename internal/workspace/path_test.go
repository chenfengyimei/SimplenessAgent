package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWithinRejectsParentEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWithin(root, filepath.Join("..", "secret.txt")); err == nil {
		t.Fatal("expected parent path escape to be rejected")
	}
	inside, err := ResolveWithin(root, "nested/file.txt")
	if err != nil {
		t.Fatalf("expected valid target: %v", err)
	}
	if filepath.Dir(filepath.Dir(inside)) != root {
		t.Fatalf("unexpected resolved path: %s", inside)
	}
}
