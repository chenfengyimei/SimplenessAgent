// Package tokenbudget centralizes small-model-safe token accounting.
package tokenbudget

import (
	"context"
	"encoding/json"
	"unicode"
	"unicode/utf8"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// EstimateText deliberately overestimates mixed Chinese/English input. CJK
// runes are counted one-for-one, other text at four runes per token, and a 15%
// safety margin is added for JSON/chat-template overhead.
func EstimateText(value string) int {
	if value == "" {
		return 0
	}
	cjk, other := 0, 0
	for _, r := range value {
		if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			cjk++
		} else {
			other++
		}
	}
	base := cjk + (other+3)/4
	if base == 0 && utf8.RuneCountInString(value) > 0 {
		base = 1
	}
	return (base*115 + 99) / 100
}

func EstimateRequest(messages []contracts.Message, tools []contracts.ToolDefinition) int {
	encoded, err := json.Marshal(struct {
		Messages []contracts.Message        `json:"messages"`
		Tools    []contracts.ToolDefinition `json:"tools,omitempty"`
	}{Messages: messages, Tools: tools})
	if err != nil {
		return 0
	}
	return EstimateText(string(encoded))
}

// Count prefers a runtime's tokenizer and falls back to the conservative
// estimator. Invalid exact counts fail closed by using the estimate.
func Count(ctx context.Context, provider contracts.ChatProvider, request contracts.TokenCountRequest) contracts.TokenCount {
	if counter, ok := provider.(contracts.TokenCounter); ok {
		if result, err := counter.CountTokens(ctx, request); err == nil && result.Tokens > 0 {
			result.Exact = true
			if result.Source == "" {
				result.Source = "provider"
			}
			return result
		}
	}
	return contracts.TokenCount{Tokens: EstimateRequest(request.Messages, request.Tools), Exact: false, Source: "cjk-safe-estimator-v1"}
}
