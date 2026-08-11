package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileCannotEscapeWorkspace(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterReadOnly(registry, root); err != nil {
		t.Fatal(err)
	}
	result, err := Invoke(registry, "read_file")(context.Background(), map[string]interface{}{"path": "../secret.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "FAILED" {
		t.Fatalf("path escape must fail, got %s", result.Status)
	}
}
