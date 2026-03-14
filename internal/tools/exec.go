package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"easybot/internal/llm"
	"easybot/internal/security"
)

type execCommandTool struct {
	pathPolicy     security.PathPolicy
	cmdPolicy      security.CommandPolicy
	audit          *security.AuditLog
	outputMaxBytes int
}

func NewExecCommandTool(pathPolicy security.PathPolicy, cmdPolicy security.CommandPolicy, audit *security.AuditLog, outputMaxBytes int) Tool {
	return execCommandTool{
		pathPolicy:     pathPolicy,
		cmdPolicy:      cmdPolicy,
		audit:          audit,
		outputMaxBytes: outputMaxBytes,
	}
}

func (t execCommandTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "exec_command",
			Description: "Run a local command without a shell. Use only when file tools are insufficient.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
					"args": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"cwd":         map[string]any{"type": "string"},
					"timeout_sec": map[string]any{"type": "integer", "minimum": 1, "maximum": 120},
				},
				"required": []string{"command"},
			},
		},
	}
}

func (t execCommandTool) SafetySummary(raw string) (bool, string) {
	args, err := parseExecArgs(raw)
	if err != nil {
		return false, "invalid command args"
	}
	decision := t.cmdPolicy.Check(append([]string{args.Command}, args.Args...))
	return decision.Safe, decision.Summary
}

func (t execCommandTool) Run(ctx context.Context, raw string) Result {
	args, err := parseExecArgs(raw)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if args.TimeoutSec <= 0 || args.TimeoutSec > 120 {
		args.TimeoutSec = 20
	}
	fullArgs := append([]string{args.Command}, args.Args...)
	decision := t.cmdPolicy.Check(fullArgs)
	if decision.Denied {
		t.audit.Write("exec_command", "denied", decision.Summary)
		return Result{Error: decision.Summary}
	}
	if strings.Contains(args.Command, string(filepath.Separator)) {
		return Result{Error: "command must be a bare executable name, not a path"}
	}
	resolvedCommand, err := exec.LookPath(args.Command)
	if err != nil {
		return Result{Error: fmt.Sprintf("command not found: %s", args.Command)}
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(args.TimeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, resolvedCommand, args.Args...)
	if args.CWD != "" {
		cwd, err := t.pathPolicy.Resolve(args.CWD)
		if err != nil {
			return Result{Error: err.Error()}
		}
		cmd.Dir = cwd
	}
	cmd.Env = safeCommandEnv()

	var stdout, stderr limitedBuffer
	stdout.maxBytes = t.outputMaxBytes
	stderr.maxBytes = t.outputMaxBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := map[string]any{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"truncated": stdout.truncated || stderr.truncated,
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			output["exit_code"] = exitErr.ExitCode()
		}
		t.audit.Write("exec_command", "error", err.Error())
		return Result{OK: false, Error: err.Error(), Data: output}
	}
	t.audit.Write("exec_command", "ok", strings.Join(fullArgs, " "))
	output["exit_code"] = 0
	return Result{OK: true, Data: output}
}

type execArgs struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	CWD        string   `json:"cwd"`
	TimeoutSec int      `json:"timeout_sec"`
}

func parseExecArgs(raw string) (execArgs, error) {
	var args execArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return execArgs{}, err
	}
	if strings.TrimSpace(args.Command) == "" {
		return execArgs{}, errors.New("command is required")
	}
	return args, nil
}

func safeCommandEnv() []string {
	keys := []string{"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR"}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

type limitedBuffer struct {
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	remaining := b.maxBytes - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return written, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, err := b.buf.Write(p)
	return written, err
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}
