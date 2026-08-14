package tool

import (
	"context"
	"strings"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// BudgetAdjustment is a model-originated request to change the context token
// limit for the current step. It lets the model request more room when it
// knows the task requires reading large files or carrying extensive context.
type BudgetAdjustment struct {
	MaxInputTokens  int
	MaxOutputTokens int
	Reason          string
}

// RegisterAdjustBudgetTool exposes a read-risk tool that lets the model
// request a different context budget for the current step. The caller decides
// whether to honor the request; the tool itself only records intent.
func RegisterAdjustBudgetTool(registry *Registry, adjust func(BudgetAdjustment) (int, int, error)) error {
	if adjust == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "budget adjust callback is required")
	}
	definition := contracts.ToolDefinition{
		Version:              contracts.SchemaVersion,
		Name:                 "adjust_context_budget",
		ToolVersion:          "1.0.0",
		Description:          "Request a different context token budget for the current step. Use when the default is too small for the task (e.g., reading large files). The system may cap or reject the request. Pass max_input_tokens and optionally max_output_tokens with a brief reason.",
		ParametersSchema:     objectSchema(map[string]interface{}{"max_input_tokens": integerSchema(), "max_output_tokens": integerSchema(), "reason": stringSchema()}, []string{"max_input_tokens"}),
		RiskClass:            contracts.RiskRead,
		RequiredCapabilities: []string{},
		MaxOutputBytes:       defaultMaxOutputBytes,
	}
	return registry.Register(definition, func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		maxInput, ok := intArg(args, "max_input_tokens")
		if !ok || maxInput <= 0 {
			return failed(started, contracts.NewError(contracts.ErrInvalidInput, "max_input_tokens must be a positive integer")), nil
		}
		maxOutput, _ := intArg(args, "max_output_tokens")
		reason, _ := args["reason"].(string)
		adj := BudgetAdjustment{MaxInputTokens: maxInput, MaxOutputTokens: maxOutput, Reason: strings.TrimSpace(reason)}
		grantedInput, grantedOutput, err := adjust(adj)
		if err != nil {
			return failed(started, err), nil
		}
		data := map[string]interface{}{
			"granted_input_tokens":  grantedInput,
			"granted_output_tokens": grantedOutput,
			"requested_input":       maxInput,
		}
		if adj.Reason != "" {
			data["reason"] = adj.Reason
		}
		summary := "context budget adjusted"
		if grantedInput < maxInput {
			summary = "context budget partially granted (capped)"
		}
		return success(started, summary, data), nil
	})
}

func intArg(args map[string]interface{}, key string) (int, bool) {
	value, ok := args[key].(float64)
	if !ok {
		return 0, false
	}
	return int(value), true
}
