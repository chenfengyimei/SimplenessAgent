package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// Intent binds approval and idempotency to a canonical tool invocation before
// any side effect may begin.
type Intent struct {
	ToolName       string
	ArgumentsHash  string
	IdempotencyKey string
	Risk           contracts.RiskClass
}

func NewIntent(toolName string, arguments map[string]interface{}, risk contracts.RiskClass, scope string) (Intent, error) {
	if strings.TrimSpace(toolName) == "" {
		return Intent{}, contracts.NewError(contracts.ErrInvalidInput, "tool name is required")
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return Intent{}, err
	}
	digest := sha256.Sum256(encoded)
	hash := "sha256:" + hex.EncodeToString(digest[:])
	keyDigest := sha256.Sum256([]byte(toolName + "\x00" + hash + "\x00" + scope))
	return Intent{ToolName: toolName, ArgumentsHash: hash, IdempotencyKey: "intent:" + hex.EncodeToString(keyDigest[:]), Risk: risk}, nil
}
