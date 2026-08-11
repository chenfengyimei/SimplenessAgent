// Package tool exposes a small, explicitly registered first-party tool surface.
package tool

import (
	"context"
	"fmt"
	"sort"

	"github.com/xm/simplenessagent/pkg/contracts"
)

type Handler func(context.Context, map[string]interface{}) (contracts.ToolResult, error)

type Registry struct {
	definitions map[string]contracts.ToolDefinition
	handlers    map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{definitions: map[string]contracts.ToolDefinition{}, handlers: map[string]Handler{}}
}
func (r *Registry) Register(definition contracts.ToolDefinition, handler Handler) error {
	if definition.Name == "" || handler == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "tool definition and handler are required")
	}
	if _, exists := r.definitions[definition.Name]; exists {
		return fmt.Errorf("tool %s is already registered", definition.Name)
	}
	r.definitions[definition.Name] = definition
	r.handlers[definition.Name] = handler
	return nil
}
func (r *Registry) Definition(name string) (contracts.ToolDefinition, bool) {
	value, ok := r.definitions[name]
	return value, ok
}
func (r *Registry) Definitions() []contracts.ToolDefinition {
	result := make([]contracts.ToolDefinition, 0, len(r.definitions))
	for _, value := range r.definitions {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
