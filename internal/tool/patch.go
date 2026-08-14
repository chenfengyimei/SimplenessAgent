package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const maxPatchBytes = 1 << 20

// RegisterApprovedApplyPatch registers apply_patch behind an approval callback.
func RegisterApprovedApplyPatch(registry *Registry, root string, approve func(map[string]interface{}) error) error {
	if approve == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "patch approval callback is required")
	}
	return registerApplyPatch(registry, root, func(args map[string]interface{}) (string, error) {
		return "", approve(args)
	})
}

// RegisterDevelopmentApplyPatch registers the direct, workspace-scoped patch
// tool used only for DEVELOPMENT tasks.
func RegisterDevelopmentApplyPatch(registry *Registry, root string, beforePatch func(map[string]interface{}) (string, error)) error {
	if beforePatch == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "development patch callback is required")
	}
	return registerApplyPatch(registry, root, beforePatch)
}

func registerApplyPatch(registry *Registry, root string, beforePatch func(map[string]interface{}) (string, error)) error {
	definition := contracts.ToolDefinition{
		Version:              contracts.SchemaVersion,
		Name:                 "apply_patch",
		ToolVersion:          "1.0.0",
		Description:          "Apply a unified diff patch to a UTF-8 workspace file. Use file_info first to get the current content hash. The patch must be a standard unified diff with --- and +++ headers and @@ hunks.",
		ParametersSchema:     objectSchema(map[string]interface{}{"path": stringSchema(), "patch": stringSchema(), "expected_content_hash": stringSchema()}, []string{"path", "patch", "expected_content_hash"}),
		RiskClass:            contracts.RiskWrite,
		RequiredCapabilities: []string{"fs.write"},
		SupportsCancel:       false,
	}
	return registry.Register(definition, func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		path, patch, expected, err := patchArguments(args)
		if err != nil {
			return failed(started, err), nil
		}
		target, err := workspace.ResolveWithin(root, path)
		if err != nil {
			return failed(started, err), nil
		}
		original, err := os.ReadFile(target)
		if err != nil {
			return failed(started, err), nil
		}
		if contentHash(original) != expected {
			return failed(started, contracts.NewError(contracts.ErrSideEffectUnknown, "expected content hash does not match current file")), nil
		}
		patched, err := applyUnifiedDiff(string(original), patch)
		if err != nil {
			return failed(started, err), nil
		}
		if contentHash([]byte(patched)) == contentHash(original) {
			return success(started, "patch applied with no changes", map[string]interface{}{"path": path, "content_hash": contentHash([]byte(patched)), "recovered": true}), nil
		}
		if err = ctx.Err(); err != nil {
			return failed(started, err), nil
		}
		toolCallID, err := beforePatch(args)
		if err != nil {
			return failed(started, err), nil
		}
		alreadyApplied, err := writeChecked(target, []byte(patched), expected)
		if err != nil {
			result := failed(started, err)
			result.ToolCallID = toolCallID
			return result, nil
		}
		summary := "patch applied"
		if alreadyApplied {
			summary = "file already contains patched content"
		}
		return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: toolCallID, Status: "SUCCEEDED", Summary: summary, Data: map[string]interface{}{"path": path, "content_hash": contentHash([]byte(patched)), "recovered": alreadyApplied}, StartedAt: started, CompletedAt: time.Now().UTC()}, nil
	})
}

// RegisterPatchProposalTool registers a propose_apply_patch that validates the
// patch but never writes. The caller persists the proposal for user approval.
func RegisterPatchProposalTool(registry *Registry, root string, request func(PatchProposalRequest) error) error {
	if request == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "patch proposal callback is required")
	}
	definition := contracts.ToolDefinition{
		Version:              contracts.SchemaVersion,
		Name:                 "propose_apply_patch",
		ToolVersion:          "1.0.0",
		Description:          "Propose applying a unified diff patch to a UTF-8 file. First call file_info and pass its exact content_hash. This only asks the user for approval; it never writes.",
		ParametersSchema:     objectSchema(map[string]interface{}{"path": stringSchema(), "patch": stringSchema(), "expected_content_hash": stringSchema()}, []string{"path", "patch", "expected_content_hash"}),
		RiskClass:            contracts.RiskWrite,
		RequiredCapabilities: []string{"fs.write", "user.prompt"},
		MaxOutputBytes:       defaultMaxOutputBytes,
	}
	return registry.Register(definition, func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		path, patch, expected, err := patchArguments(args)
		if err != nil {
			return failed(started, err), nil
		}
		target, err := workspace.ResolveWithin(root, path)
		if err != nil {
			return failed(started, err), nil
		}
		original, err := os.ReadFile(target)
		if err != nil {
			return failed(started, err), nil
		}
		if contentHash(original) != expected {
			return failed(started, contracts.NewError(contracts.ErrSideEffectUnknown, "expected content hash does not match current file")), nil
		}
		patched, err := applyUnifiedDiff(string(original), patch)
		if err != nil {
			return failed(started, err), nil
		}
		if err = request(PatchProposalRequest{Writes: []WriteProposal{{Path: path, Content: patched, ExpectedContentHash: expected}}}); err != nil {
			return failed(started, err), nil
		}
		return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: "pending-approval", Status: "WAITING_APPROVAL", Summary: "patch application is waiting for user approval", Data: map[string]interface{}{"path": path, "content_hash": contentHash([]byte(patched)), "bytes": len(patched)}, StartedAt: started, CompletedAt: time.Now().UTC()}, nil
	})
}

// PatchProposalRequest wraps the resolved write so the existing approval
// pipeline handles patch proposals identically to full-file writes.
type PatchProposalRequest struct {
	Writes []WriteProposal
}

func patchArguments(args map[string]interface{}) (string, string, string, error) {
	path, pathOK := args["path"].(string)
	patch, patchOK := args["patch"].(string)
	expected, expectedOK := args["expected_content_hash"].(string)
	if !pathOK || !patchOK || !expectedOK || strings.TrimSpace(path) == "" || patch == "" || expected == "" {
		return "", "", "", contracts.NewError(contracts.ErrInvalidInput, "path, patch and expected_content_hash are required strings")
	}
	if len(patch) > maxPatchBytes {
		return "", "", "", contracts.NewError(contracts.ErrBudgetExceeded, "patch content exceeds the safety limit")
	}
	return path, patch, expected, nil
}

// applyUnifiedDiff applies a standard unified diff patch to the original text
// and returns the patched result. It supports context lines, additions and
// deletions. Hunk headers (@@) must have accurate line numbers.
func applyUnifiedDiff(original, patch string) (string, error) {
	originalLines := splitLines(original)
	patchLines := splitLines(patch)
	if len(patchLines) < 2 {
		return "", contracts.NewError(contracts.ErrInvalidInput, "patch must contain at least a header and one hunk")
	}
	result := make([]string, 0, len(originalLines))
	originalIdx := 0
	patchIdx := 0
	for patchIdx < len(patchLines) {
		line := patchLines[patchIdx]
		if strings.HasPrefix(line, "---") {
			patchIdx++
			continue
		}
		if strings.HasPrefix(line, "+++") {
			patchIdx++
			continue
		}
		if strings.HasPrefix(line, "@@") {
			oldStart, oldCount, err := parseHunkHeader(line)
			if err != nil {
				return "", err
			}
			startIdx := oldStart - 1
			if startIdx < 0 || startIdx > len(originalLines) {
				return "", contracts.NewError(contracts.ErrInvalidInput, fmt.Sprintf("hunk start line %d is out of range", oldStart))
			}
			if startIdx < originalIdx {
				return "", contracts.NewError(contracts.ErrInvalidInput, fmt.Sprintf("hunk start %d overlaps with previous hunk", oldStart))
			}
			result = append(result, originalLines[originalIdx:startIdx]...)
			originalIdx = startIdx
			patchIdx++
			consumed := 0
			for patchIdx < len(patchLines) {
				patchLine := patchLines[patchIdx]
				if strings.HasPrefix(patchLine, "@@") {
					break
				}
				if consumed >= oldCount && !strings.HasPrefix(patchLine, "+") {
					break
				}
				switch {
				case strings.HasPrefix(patchLine, " "):
					if originalIdx < len(originalLines) {
						result = append(result, originalLines[originalIdx])
						originalIdx++
					}
					consumed++
				case strings.HasPrefix(patchLine, "-"):
					originalIdx++
					consumed++
				case strings.HasPrefix(patchLine, "+"):
					result = append(result, patchLine[1:])
				case patchLine == "":
					if originalIdx < len(originalLines) {
						result = append(result, originalLines[originalIdx])
						originalIdx++
					}
					consumed++
				default:
					return "", contracts.NewError(contracts.ErrInvalidInput, fmt.Sprintf("invalid patch line: %q", patchLine))
				}
				patchIdx++
			}
			continue
		}
		if line == "" {
			patchIdx++
			continue
		}
		return "", contracts.NewError(contracts.ErrInvalidInput, fmt.Sprintf("unexpected patch line: %q", line))
	}
	result = append(result, originalLines[originalIdx:]...)
	return strings.Join(result, "\n") + func() string {
		if len(result) > 0 && !strings.HasSuffix(original, "\n") {
			return ""
		}
		return ""
	}(), nil
}

func parseHunkHeader(line string) (start, count int, err error) {
	if !strings.HasPrefix(line, "@@") {
		return 0, 0, contracts.NewError(contracts.ErrInvalidInput, "expected @@ hunk header")
	}
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 2 {
		return 0, 0, contracts.NewError(contracts.ErrInvalidInput, "malformed hunk header")
	}
	rangeStr := strings.TrimSpace(parts[1])
	rangeParts := strings.SplitN(rangeStr, ",", 2)
	if len(rangeParts) == 0 {
		return 0, 0, contracts.NewError(contracts.ErrInvalidInput, "hunk header missing old range")
	}
	var s int
	if _, err = fmt.Sscanf(rangeParts[0], "-%d", &s); err != nil {
		return 0, 0, contracts.NewError(contracts.ErrInvalidInput, "invalid hunk old start")
	}
	if len(rangeParts) > 1 {
		if _, err = fmt.Sscanf(rangeParts[1], "%d", &count); err != nil {
			count = 1
		}
	} else {
		count = 1
	}
	return s, count, nil
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

var _ = filepath.Join
var _ = sha256.Sum256
var _ = hex.EncodeToString
