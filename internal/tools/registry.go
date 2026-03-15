package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/I-can-do-fourier/EasyBot/internal/llm"
	"github.com/I-can-do-fourier/EasyBot/internal/security"
)

type Result struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func (r Result) Status() string {
	if r.OK {
		return "ok"
	}
	return "error"
}

type Tool interface {
	Spec() llm.ToolSpec
	SafetySummary(rawArgs string) (bool, string)
	Run(ctx context.Context, rawArgs string) Result
}

type Registry struct {
	tools map[string]Tool
	names []string
}

func NewRegistry(pathPolicy security.PathPolicy, cmdPolicy security.CommandPolicy, audit *security.AuditLog, toolOutputMaxBytes int) *Registry {
	reg := &Registry{tools: map[string]Tool{}}
	for _, tool := range []Tool{
		NewListDirTool(pathPolicy, audit),
		NewReadFileTool(pathPolicy, audit),
		NewReadPdfTool(pathPolicy, audit),
		NewFindFilesTool(pathPolicy, audit),
		NewSearchFileContentTool(pathPolicy, audit),
		NewStatFileTool(pathPolicy, audit),
		NewWriteFileTool(pathPolicy, audit),
		NewExecCommandTool(pathPolicy, cmdPolicy, audit, toolOutputMaxBytes),
	} {
		reg.tools[tool.Spec().Function.Name] = tool
		reg.names = append(reg.names, tool.Spec().Function.Name)
	}
	sort.Strings(reg.names)
	return reg
}

func (r *Registry) Specs() []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, len(r.names))
	for _, name := range r.names {
		out = append(out, r.tools[name].Spec())
	}
	return out
}

func (r *Registry) SafetyCheck(call llm.ToolCall) (bool, string) {
	tool, ok := r.tools[call.Function.Name]
	if !ok {
		return false, fmt.Sprintf("unknown tool %q", call.Function.Name)
	}
	return tool.SafetySummary(call.Function.Arguments)
}

func (r *Registry) Execute(ctx context.Context, call llm.ToolCall) Result {
	tool, ok := r.tools[call.Function.Name]
	if !ok {
		return Result{OK: false, Error: "unknown tool"}
	}
	return tool.Run(ctx, call.Function.Arguments)
}

func decodeArgs(raw string, out any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}
