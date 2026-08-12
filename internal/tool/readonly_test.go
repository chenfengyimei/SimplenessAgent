package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestFileInfoAndLineReadExposeStableHash(t *testing.T) {
	root := t.TempDir()
	contents := []byte("one\ntwo\nthree\n")
	if err := os.WriteFile(filepath.Join(root, "note.txt"), contents, 0o644); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterReadOnly(registry, root); err != nil {
		t.Fatal(err)
	}
	info, err := Invoke(registry, "file_info")(context.Background(), map[string]interface{}{"path": "note.txt"})
	if err != nil || info.Status != "SUCCEEDED" || info.Data["content_hash"] != testHash(contents) {
		t.Fatal(info, err)
	}
	read, err := Invoke(registry, "read_file")(context.Background(), map[string]interface{}{"path": "note.txt", "start_line": float64(2), "end_line": float64(3)})
	if err != nil || read.Status != "SUCCEEDED" || read.Data["content"] != "two\nthree" || read.Data["content_hash"] != testHash(contents) {
		t.Fatal(read, err)
	}
}

func TestReadOnlyCommandsDoNotExposeShell(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterReadOnly(registry, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"run_go_test", "run_go_vet", "git_status", "git_diff"} {
		definition, found := registry.Definition(name)
		if !found || definition.RiskClass != "READ" || definition.ParametersSchema == nil {
			t.Fatalf("missing safe command tool %s: %#v", name, definition)
		}
	}
	if _, found := registry.Definition("execute_command"); found {
		t.Fatal("arbitrary shell execution must not be registered")
	}
}

func testHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
