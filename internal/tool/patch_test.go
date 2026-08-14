package tool

import (
	"testing"
)

func TestApplyUnifiedDiff_SingleLineChange(t *testing.T) {
	original := "line1\nline2\nline3"
	patch := "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+line2 modified\n line3"
	result, err := applyUnifiedDiff(original, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "line1\nline2 modified\nline3"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestApplyUnifiedDiff_AddLine(t *testing.T) {
	original := "line1\nline3"
	patch := "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,3 @@\n line1\n+line2\n line3"
	result, err := applyUnifiedDiff(original, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "line1\nline2\nline3"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestApplyUnifiedDiff_RemoveLine(t *testing.T) {
	original := "line1\nline2\nline3"
	patch := "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,2 @@\n line1\n-line2\n line3"
	result, err := applyUnifiedDiff(original, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "line1\nline3"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestApplyUnifiedDiff_MultipleHunks(t *testing.T) {
	original := "a\nb\nc\nd\ne\nf"
	patch := "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n@@ -5,2 +5,2 @@\n e\n-f\n+F"
	result, err := applyUnifiedDiff(original, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "a\nB\nc\nd\ne\nF"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestApplyUnifiedDiff_InvalidPatch(t *testing.T) {
	_, err := applyUnifiedDiff("hello", "not a patch")
	if err == nil {
		t.Error("expected error for invalid patch")
	}
}

func TestPatchArguments(t *testing.T) {
	_, _, _, err := patchArguments(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing arguments")
	}
	path, patch, hash, err := patchArguments(map[string]interface{}{
		"path":                  "test.txt",
		"patch":                 "--- a\n+++ b\n@@ -1,1 +1,1 @@\n-old\n+new",
		"expected_content_hash": "sha256:abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "test.txt" || patch == "" || hash != "sha256:abc" {
		t.Error("unexpected argument values")
	}
}
