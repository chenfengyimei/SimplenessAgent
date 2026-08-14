package evals_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluationJSONFilesAreValid(t *testing.T) {
	patterns := []string{"suites/*.json", "baselines/*.json", "fixtures/*.json"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Fatalf("no evaluation files matched %s", pattern)
		}
		for _, name := range matches {
			content, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(content) {
				t.Errorf("%s is not valid JSON", name)
			}
		}
	}
}
