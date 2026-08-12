package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		{contracts.ToolDefinition{Version: 1, Name: "list_files", ToolVersion: "1.0.0", Description: "List files under an authorized workspace path.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema(), "limit": integerSchema()}, []string{"path"}), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, listFiles(root)},
		{contracts.ToolDefinition{Version: 1, Name: "file_info", ToolVersion: "1.0.0", Description: "Get a workspace file's existence, size and content hash before changing it.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema()}, []string{"path"}), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, fileInfo(root)},
		{contracts.ToolDefinition{Version: 1, Name: "read_file", ToolVersion: "1.1.0", Description: "Read a UTF-8 text file inside the authorized workspace, optionally by line range.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema(), "start_line": integerSchema(), "end_line": integerSchema()}, []string{"path"}), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, readFile(root)},
		{contracts.ToolDefinition{Version: 1, Name: "search_text", ToolVersion: "1.0.0", Description: "Search text files inside the authorized workspace.", ParametersSchema: objectSchema(map[string]interface{}{"query": stringSchema(), "path": stringSchema(), "limit": integerSchema()}, []string{"query", "path"}), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: defaultMaxOutputBytes}, searchText(root)},
	}
	for _, item := range definitions {
		if err := registry.Register(item.definition, item.handler); err != nil {
			return err
		}
	}
	return RegisterReadOnlyCommands(registry, root)
}

func objectSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	fields := make([]interface{}, len(required))
	for index, name := range required {
		fields[index] = name
	}
	return map[string]interface{}{"type": "object", "properties": properties, "required": fields, "additionalProperties": false}
}
func stringSchema() map[string]interface{}  { return map[string]interface{}{"type": "string"} }
func integerSchema() map[string]interface{} { return map[string]interface{}{"type": "integer"} }
func arraySchema(items map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "array", "items": items}
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
				if ignoredDirectory(entry.Name()) {
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

func fileInfo(root string) Handler {
	return func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		relative, _ := args["path"].(string)
		target, err := workspace.ResolveWithin(root, relative)
		if err != nil {
			return failed(started, err), nil
		}
		info, err := os.Stat(target)
		if os.IsNotExist(err) {
			return success(started, "path does not exist", map[string]interface{}{"path": relative, "exists": false, "content_hash": contentHash(nil)}), nil
		}
		if err != nil {
			return failed(started, err), nil
		}
		data := map[string]interface{}{"path": relative, "exists": true, "is_directory": info.IsDir(), "size_bytes": info.Size()}
		if !info.IsDir() {
			contents, readErr := os.ReadFile(target)
			if readErr != nil {
				return failed(started, readErr), nil
			}
			data["content_hash"] = contentHash(contents)
		}
		return success(started, "file information read", data), nil
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
		contentHash := contentHash(contents)
		if start, end, selected := lineRange(args); selected {
			lines := strings.Split(string(contents), "\n")
			if start <= 0 || end < start || start > len(lines) {
				return failed(started, contracts.NewError(contracts.ErrInvalidInput, "requested line range is outside the file")), nil
			}
			if end > len(lines) {
				end = len(lines)
			}
			contents = []byte(strings.Join(lines[start-1:end], "\n"))
			return success(started, "file lines read", map[string]interface{}{"content": string(contents), "content_hash": contentHash, "start_line": start, "end_line": end, "truncated": false}), nil
		}
		if len(contents) > defaultMaxOutputBytes {
			contents = append(contents[:defaultMaxOutputBytes/2], contents[len(contents)-defaultMaxOutputBytes/2:]...)
			return success(started, "file output truncated", map[string]interface{}{"content": string(contents), "content_hash": contentHash, "truncated": true}), nil
		}
		return success(started, "file read", map[string]interface{}{"content": string(contents), "content_hash": contentHash, "truncated": false}), nil
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
				if ignoredDirectory(entry.Name()) {
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

func ignoredDirectory(name string) bool {
	return name == ".git" || name == "node_modules" || name == ".idea" || name == ".simpleness"
}

func lineRange(arguments map[string]interface{}) (int, int, bool) {
	start, hasStart := integer(arguments["start_line"])
	end, hasEnd := integer(arguments["end_line"])
	return start, end, hasStart || hasEnd
}

func integer(value interface{}) (int, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func contentHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
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

func failedWithData(started time.Time, err error, data map[string]interface{}) contracts.ToolResult {
	result := failed(started, err)
	result.Data = data
	return result
}
