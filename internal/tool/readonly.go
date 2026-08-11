package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const defaultMaxOutputBytes = 32 * 1024

func RegisterReadOnly(registry *Registry, root string) error {
	definitions := []struct {
		definition contracts.ToolDefinition
		handler    Handler
	}{
		{contracts.ToolDefinition{Version: 1, Name: "list_files", ToolVersion: "1.0.0", Description: "List files under an authorized workspace path.", RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, listFiles(root)},
		{contracts.ToolDefinition{Version: 1, Name: "read_file", ToolVersion: "1.0.0", Description: "Read a UTF-8 text file inside the authorized workspace.", RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, readFile(root)},
		{contracts.ToolDefinition{Version: 1, Name: "search_text", ToolVersion: "1.0.0", Description: "Search text files inside the authorized workspace.", RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, searchText(root)},
	}
	for _, item := range definitions {
		if err := registry.Register(item.definition, item.handler); err != nil {
			return err
		}
	}
	return nil
}

func listFiles(root string) Handler {
	return func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		relative, _ := args["path"].(string)
		target, err := workspace.ResolveWithin(root, relative)
		if err != nil {
			return failed(started, err), nil
		}
		max := limit(args, 200)
		files := []string{}
		err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if path == target {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			files = append(files, rel)
			if len(files) >= max {
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return failed(started, err), nil
		}
		sort.Strings(files)
		return success(started, fmt.Sprintf("listed %d files", len(files)), map[string]interface{}{"files": files, "truncated": len(files) >= max}), nil
	}
}

func readFile(root string) Handler {
	return func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		relative, _ := args["path"].(string)
		target, err := workspace.ResolveWithin(root, relative)
		if err != nil {
			return failed(started, err), nil
		}
		contents, err := os.ReadFile(target)
		if err != nil {
			return failed(started, err), nil
		}
		if len(contents) > defaultMaxOutputBytes {
			contents = append(contents[:defaultMaxOutputBytes/2], contents[len(contents)-defaultMaxOutputBytes/2:]...)
			return success(started, "file output truncated", map[string]interface{}{"content": string(contents), "truncated": true}), nil
		}
		return success(started, "file read", map[string]interface{}{"content": string(contents), "truncated": false}), nil
	}
}

func searchText(root string) Handler {
	return func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		needle, _ := args["query"].(string)
		if strings.TrimSpace(needle) == "" {
			return failed(started, contracts.NewError(contracts.ErrInvalidInput, "query is required")), nil
		}
		relative, _ := args["path"].(string)
		target, err := workspace.ResolveWithin(root, relative)
		if err != nil {
			return failed(started, err), nil
		}
		max := limit(args, 100)
		matches := []map[string]interface{}{}
		err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil || len(contents) > 1024*1024 {
				return nil
			}
			for lineNo, line := range strings.Split(string(contents), "\n") {
				if strings.Contains(line, needle) {
					rel, _ := filepath.Rel(root, path)
					matches = append(matches, map[string]interface{}{"path": rel, "line": lineNo + 1, "text": line})
					if len(matches) >= max {
						return filepath.SkipAll
					}
				}
			}
			return nil
		})
		if err != nil {
			return failed(started, err), nil
		}
		return success(started, fmt.Sprintf("found %d matches", len(matches)), map[string]interface{}{"matches": matches, "truncated": len(matches) >= max}), nil
	}
}

func limit(arguments map[string]interface{}, fallback int) int {
	if value, ok := arguments["limit"].(float64); ok && value > 0 && value < float64(fallback) {
		return int(value)
	}
	return fallback
}
func success(started time.Time, summary string, data map[string]interface{}) contracts.ToolResult {
	return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: task.NewID("tcall"), Status: "SUCCEEDED", Summary: summary, Data: data, StartedAt: started, CompletedAt: time.Now().UTC()}
}
func failed(started time.Time, err error) contracts.ToolResult {
	code := string(contracts.ErrInvalidInput)
	if domain, ok := err.(*contracts.Error); ok {
		code = string(domain.Code)
	}
	return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: task.NewID("tcall"), Status: "FAILED", Summary: err.Error(), Error: &contracts.ToolError{Code: code, Message: err.Error()}, StartedAt: started, CompletedAt: time.Now().UTC()}
}
