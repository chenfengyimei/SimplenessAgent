package policy

import (
	"github.com/xm/simplenessagent/pkg/contracts"
	"testing"
)

func TestNewIntentCanonicalizesArguments(t *testing.T) {
	left, _ := NewIntent("write", map[string]interface{}{"b": 2, "a": "x"}, contracts.RiskWrite, "step")
	right, _ := NewIntent("write", map[string]interface{}{"a": "x", "b": 2}, contracts.RiskWrite, "step")
	if left.ArgumentsHash != right.ArgumentsHash || left.IdempotencyKey != right.IdempotencyKey {
		t.Fatalf("intent is not canonical: %#v %#v", left, right)
	}
}
