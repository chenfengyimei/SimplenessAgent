package diagnostics

import "testing"

func TestLoggerWritesQueriesAndRedactsSecrets(t *testing.T) {
	logger, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("desktop", "started", map[string]string{"deployment_id": "dep-1", "api_key": "must-not-appear"})
	items, err := logger.Query(10)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if items[0].Fields["api_key"] != "[REDACTED]" || items[0].Fields["deployment_id"] != "dep-1" {
		t.Fatalf("unexpected fields: %#v", items[0].Fields)
	}
}
