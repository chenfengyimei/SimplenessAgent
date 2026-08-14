package tool

import (
	"context"
	"strings"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// UserQuestion is a model-originated question that pauses execution until the
// user provides an answer. It is persisted as a conversation message.
type UserQuestion struct {
	Question string
	Options  []string
	Context  string
}

// RegisterAskUserTool exposes a read-risk tool that lets the model ask the
// user a clarifying question. The worker pauses; the caller must persist the
// question and later resume the task with the user's answer.
func RegisterAskUserTool(registry *Registry, ask func(UserQuestion) error) error {
	if ask == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "ask callback is required")
	}
	definition := contracts.ToolDefinition{
		Version:              contracts.SchemaVersion,
		Name:                 "ask_user",
		ToolVersion:          "1.0.0",
		Description:          "Ask the user a clarifying question. Use only when the answer would materially change the result. Provide context explaining why the question is needed.",
		ParametersSchema:     objectSchema(map[string]interface{}{"question": stringSchema(), "options": arraySchema(stringSchema()), "context": stringSchema()}, []string{"question"}),
		RiskClass:            contracts.RiskRead,
		RequiredCapabilities: []string{"user.prompt"},
		MaxOutputBytes:       defaultMaxOutputBytes,
	}
	return registry.Register(definition, func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		question, questionOK := args["question"].(string)
		if !questionOK || strings.TrimSpace(question) == "" {
			return failed(started, contracts.NewError(contracts.ErrInvalidInput, "question is a required non-empty string")), nil
		}
		uq := UserQuestion{Question: strings.TrimSpace(question)}
		if rawOptions, ok := args["options"].([]interface{}); ok {
			for _, raw := range rawOptions {
				if opt, ok := raw.(string); ok && strings.TrimSpace(opt) != "" {
					uq.Options = append(uq.Options, strings.TrimSpace(opt))
				}
			}
		}
		if ctx, ok := args["context"].(string); ok {
			uq.Context = strings.TrimSpace(ctx)
		}
		if err := ask(uq); err != nil {
			return failed(started, err), nil
		}
		return contracts.ToolResult{
			Version:     contracts.SchemaVersion,
			ToolCallID:  "waiting-user",
			Status:      "WAITING_USER",
			Summary:     "question sent to user; execution paused",
			Data:        map[string]interface{}{"question": uq.Question, "options": uq.Options},
			StartedAt:   started,
			CompletedAt: time.Now().UTC(),
		}, nil
	})
}
