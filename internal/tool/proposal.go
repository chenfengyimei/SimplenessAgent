package tool

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const maxProposedWriteBytes = 1 << 20

// WriteProposal is a fully specified write that has not changed the
// workspace. The application persists it as a reviewable approval request.
type WriteProposal struct {
	Path                string
	Content             string
	ExpectedContentHash string
}

// RegisterWriteProposalTools registers model-facing write proposals. These
// handlers validate the file version but never write; the caller must persist
// the proposal and obtain a parameter-bound user approval before execution.
func RegisterWriteProposalTools(registry *Registry, root string, request func(WriteProposal) error) error {
	if request == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "write proposal callback is required")
	}
	definitions := []struct {
		definition contracts.ToolDefinition
		handler    Handler
	}{
		{contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "propose_write_file", ToolVersion: "1.0.0", Description: "Propose one complete UTF-8 file write. First call file_info and pass its exact content_hash. This only asks the user for approval; it never writes.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema(), "content": stringSchema(), "expected_content_hash": stringSchema()}, []string{"path", "content", "expected_content_hash"}), RiskClass: contracts.RiskWrite, RequiredCapabilities: []string{"fs.write", "user.prompt"}, MaxOutputBytes: defaultMaxOutputBytes}, proposeWrite(root, request)},
		{contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "propose_text_replace", ToolVersion: "1.0.0", Description: "Propose replacing exactly one literal text fragment in a UTF-8 file. First call file_info and pass its exact content_hash. This only asks the user for approval; it never writes.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema(), "old_text": stringSchema(), "new_text": stringSchema(), "expected_content_hash": stringSchema()}, []string{"path", "old_text", "new_text", "expected_content_hash"}), RiskClass: contracts.RiskWrite, RequiredCapabilities: []string{"fs.write", "user.prompt"}, MaxOutputBytes: defaultMaxOutputBytes}, proposeReplace(root, request)},
	}
	for _, item := range definitions {
		if err := registry.Register(item.definition, item.handler); err != nil {
			return err
		}
	}
	return nil
}

func proposeWrite(root string, request func(WriteProposal) error) Handler {
	return func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		proposal, err := proposalArguments(args)
		if err != nil {
			return failed(started, err), nil
		}
		if err = validateProposalTarget(root, proposal); err != nil {
			return failed(started, err), nil
		}
		if err = request(proposal); err != nil {
			return failed(started, err), nil
		}
		return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: "pending-approval", Status: "WAITING_APPROVAL", Summary: "file write is waiting for user approval", Data: map[string]interface{}{"path": proposal.Path, "content_hash": contentHash([]byte(proposal.Content)), "bytes": len(proposal.Content)}, StartedAt: started, CompletedAt: time.Now().UTC()}, nil
	}
}

func proposeReplace(root string, request func(WriteProposal) error) Handler {
	return func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		path, pathOK := args["path"].(string)
		oldText, oldOK := args["old_text"].(string)
		newText, newOK := args["new_text"].(string)
		expected, expectedOK := args["expected_content_hash"].(string)
		if !pathOK || !oldOK || !newOK || !expectedOK || strings.TrimSpace(path) == "" || oldText == "" || expected == "" {
			return failed(started, contracts.NewError(contracts.ErrInvalidInput, "path, non-empty old_text, new_text and expected_content_hash are required")), nil
		}
		target, err := workspace.ResolveWithin(root, path)
		if err != nil {
			return failed(started, err), nil
		}
		contents, err := os.ReadFile(target)
		if err != nil {
			return failed(started, err), nil
		}
		if contentHash(contents) != expected {
			return failed(started, contracts.NewError(contracts.ErrSideEffectUnknown, "expected content hash does not match current file")), nil
		}
		if strings.Count(string(contents), oldText) != 1 {
			return failed(started, contracts.NewError(contracts.ErrInvalidInput, "old_text must occur exactly once in the current file")), nil
		}
		proposal := WriteProposal{Path: path, Content: strings.Replace(string(contents), oldText, newText, 1), ExpectedContentHash: expected}
		if len(proposal.Content) > maxProposedWriteBytes {
			return failed(started, contracts.NewError(contracts.ErrBudgetExceeded, "proposed file content exceeds the safety limit")), nil
		}
		if err = request(proposal); err != nil {
			return failed(started, err), nil
		}
		return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: "pending-approval", Status: "WAITING_APPROVAL", Summary: "text replacement is waiting for user approval", Data: map[string]interface{}{"path": path, "content_hash": contentHash([]byte(proposal.Content)), "bytes": len(proposal.Content)}, StartedAt: started, CompletedAt: time.Now().UTC()}, nil
	}
}

func proposalArguments(args map[string]interface{}) (WriteProposal, error) {
	path, pathOK := args["path"].(string)
	content, contentOK := args["content"].(string)
	expected, expectedOK := args["expected_content_hash"].(string)
	if !pathOK || !contentOK || !expectedOK || strings.TrimSpace(path) == "" || expected == "" {
		return WriteProposal{}, contracts.NewError(contracts.ErrInvalidInput, "path, content and expected_content_hash are required strings")
	}
	if len(content) > maxProposedWriteBytes {
		return WriteProposal{}, contracts.NewError(contracts.ErrBudgetExceeded, "proposed file content exceeds the safety limit")
	}
	return WriteProposal{Path: path, Content: content, ExpectedContentHash: expected}, nil
}

func validateProposalTarget(root string, proposal WriteProposal) error {
	target, err := workspace.ResolveWithin(root, proposal.Path)
	if err != nil {
		return err
	}
	contents, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if contentHash(contents) != proposal.ExpectedContentHash {
		return contracts.NewError(contracts.ErrSideEffectUnknown, "expected content hash does not match current file")
	}
	return nil
}
