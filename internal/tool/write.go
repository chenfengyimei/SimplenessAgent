package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

// RegisterApprovedWriteFile is intentionally opt-in. The approval callback must
// record/consume a parameter-bound ticket before this handler writes anything.
func RegisterApprovedWriteFile(registry *Registry, root string, approve func(map[string]interface{}) error) error {
	if approve == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "write approval callback is required")
	}
	definition := contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "write_file", ToolVersion: "1.0.0", Description: "Write a UTF-8 file after parameter-bound approval.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema(), "content": stringSchema(), "expected_content_hash": stringSchema()}, []string{"path", "content", "expected_content_hash"}), RiskClass: contracts.RiskWrite, RequiredCapabilities: []string{"fs.write"}, SupportsCancel: false}
	return registry.Register(definition, func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		path, content, expected, err := writeArguments(args)
		if err != nil {
			return failed(started, err), nil
		}
		target, err := workspace.ResolveWithin(root, path)
		if err != nil {
			return failed(started, err), nil
		}
		alreadyApplied, err := checkExpectedContent(target, expected, hash([]byte(content)))
		if err != nil {
			return failed(started, err), nil
		}
		if alreadyApplied {
			return success(started, "file already contains requested content", map[string]interface{}{"path": path, "content_hash": hash([]byte(content)), "recovered": true}), nil
		}
		if err = ctx.Err(); err != nil {
			return failed(started, err), nil
		}
		if err = approve(args); err != nil {
			return failed(started, err), nil
		}
		alreadyApplied, err = writeChecked(target, []byte(content), expected)
		if err != nil {
			return failed(started, err), nil
		}
		summary := "file written"
		if alreadyApplied {
			summary = "file already contains requested content"
		}
		return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: task.NewID("tcall"), Status: "SUCCEEDED", Summary: summary, Data: map[string]interface{}{"path": path, "content_hash": hash([]byte(content)), "recovered": alreadyApplied}, StartedAt: started, CompletedAt: time.Now().UTC()}, nil
	})
}

func writeArguments(args map[string]interface{}) (string, string, string, error) {
	path, pathOK := args["path"].(string)
	content, contentOK := args["content"].(string)
	expected, expectedOK := args["expected_content_hash"].(string)
	if !pathOK || !contentOK || !expectedOK || path == "" || expected == "" {
		return "", "", "", contracts.NewError(contracts.ErrInvalidInput, "path, content and expected_content_hash are required strings")
	}
	return path, content, expected, nil
}

func writeChecked(target string, content []byte, expected string) (bool, error) {
	alreadyApplied, err := checkExpectedContent(target, expected, hash(content))
	if err != nil || alreadyApplied {
		return alreadyApplied, err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(parent, ".simpleness-write-*")
	if err != nil {
		return false, err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(content); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	return false, os.Rename(name, target)
}

func checkExpectedContent(target, expected, desired string) (bool, error) {
	current, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	actual := hash(current)
	if actual == desired {
		return true, nil
	}
	if expected != actual {
		return false, contracts.NewError(contracts.ErrSideEffectUnknown, "expected content hash does not match current file")
	}
	return false, nil
}
func hash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
