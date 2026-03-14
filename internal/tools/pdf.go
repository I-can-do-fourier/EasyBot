// write a tool to extract pdf
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"easybot/internal/llm"
	"easybot/internal/security"
)

type readPdfTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewReadPdfTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return readPdfTool{policy: policy, audit: audit}
}

func (t readPdfTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "read_pdf",
			Description: "Extract text content from a PDF file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t readPdfTool) SafetySummary(raw string) (bool, string) {
	return true, "read-only PDF extraction"
}

func (t readPdfTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if !fileExists(path) {
		return Result{Error: fmt.Sprintf("file not found: %s", path)}
	}
	cmd := exec.Command("pdftotext", path, "-")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return Result{Error: fmt.Sprintf("failed to extract PDF content: %s", stderr.String())}
	}
	t.audit.Write("read_pdf", "ok", path)
	return Result{OK: true, Data: out.String()}
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := exec.Command("test", "-f", path).Output()
	return err == nil
}
