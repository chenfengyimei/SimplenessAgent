package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestApprovedWriteFileChecksApprovalAndHash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	approved := 0
	if err := RegisterApprovedWriteFile(r, root, func(map[string]interface{}) error { approved++; return nil }); err != nil {
		t.Fatal(err)
	}
	result, err := Invoke(r, "write_file")(context.Background(), map[string]interface{}{"path": "a.txt", "content": "new", "expected_content_hash": hash([]byte("old"))})
	if err != nil || result.Status != "SUCCEEDED" {
		t.Fatal(result, err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "new" || approved != 1 {
		t.Fatal(string(data), approved)
	}
	result, _ = Invoke(r, "write_file")(context.Background(), map[string]interface{}{"path": "a.txt", "content": "bad", "expected_content_hash": hash([]byte("old"))})
	if result.Status != "FAILED" {
		t.Fatal(result)
	}
	if approved != 1 {
		t.Fatalf("approval callback called %d times, want 1", approved)
	}
	result, err = Invoke(r, "write_file")(context.Background(), map[string]interface{}{"path": "a.txt", "content": "new", "expected_content_hash": hash([]byte("old"))})
	if err != nil || result.Status != "SUCCEEDED" || result.Data["recovered"] != true {
		t.Fatal(result, err)
	}
	if approved != 1 {
		t.Fatalf("recovery consumed approval: %d", approved)
	}
}

func TestApprovedWriteFileDoesNotWriteWhenApprovalRejected(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := RegisterApprovedWriteFile(r, root, func(map[string]interface{}) error { return os.ErrPermission }); err != nil {
		t.Fatal(err)
	}
	result, err := Invoke(r, "write_file")(context.Background(), map[string]interface{}{"path": "a.txt", "content": "new", "expected_content_hash": hash([]byte("old"))})
	if err != nil || result.Status != "FAILED" {
		t.Fatal(result, err)
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil || string(data) != "old" {
		t.Fatal(string(data), readErr)
	}
}
