package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

type Tool interface {
	Name() Intent
	Execute(ctx context.Context, arguments json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
	Answer string
	Data   any
}

type ToolRegistry struct {
	tools map[Intent]Tool
}

func NewToolRegistry(tools ...Tool) (*ToolRegistry, error) {
	registered := make(map[Intent]Tool, len(tools))
	for _, tool := range tools {
		if isNilInterface(tool) {
			return nil, errors.New("register tool: tool must not be nil")
		}
		name := tool.Name()
		if !name.Valid() {
			return nil, fmt.Errorf("register tool: unsupported tool name %q", name)
		}
		if _, exists := registered[name]; exists {
			return nil, fmt.Errorf("register tool: duplicate tool name %q", name)
		}
		registered[name] = tool
	}
	return &ToolRegistry{tools: registered}, nil
}

func (r *ToolRegistry) Execute(
	ctx context.Context,
	intent Intent,
	arguments json.RawMessage,
) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	if r == nil {
		return ToolResult{}, WrapError(CodeInternal, errors.New("tool registry is nil"))
	}
	tool, ok := r.tools[intent]
	if !ok {
		return ToolResult{}, NewError(CodeUnknownIntent)
	}
	return tool.Execute(ctx, arguments)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
