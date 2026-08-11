package tool

import (
	"context"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// Invoke is the controlled Registry invocation point. A later Worker will add
// schema, policy, write-ahead and approval middleware ahead of this boundary.
func Invoke(registry *Registry, name string) Handler {
	return func(ctx context.Context, arguments map[string]interface{}) (contracts.ToolResult, error) {
		handler, exists := registry.handlers[name]
		if !exists {
			return contracts.ToolResult{}, contracts.NewError(contracts.ErrNotFound, "tool is not registered")
		}
		return handler(ctx, arguments)
	}
}
