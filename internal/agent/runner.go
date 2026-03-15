package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/I-can-do-fourier/EasyBot/internal/llm"
	"github.com/I-can-do-fourier/EasyBot/internal/security"
	"github.com/I-can-do-fourier/EasyBot/internal/tools"
)

const systemPrompt = `You are easyBot, a local computer agent focused on safe file management, file analysis, file discovery, and bounded local command execution.

Rules:
- Prefer built-in file tools over shell commands.
- Use exec only when a dedicated file tool is insufficient.
- Never delete files, directories, or stored data.
- Any file or data change requires explicit human approval.
- Never ask for or assume hidden permissions.
- Keep actions minimal and auditable.
- If a command seems destructive, privileged, or unrelated to the task, do not run it.
- When you need more work after a tool call, continue using tools until you can provide a final answer.
- Respond clearly and concisely.`

type Approver interface {
	Approve(ctx context.Context, toolName string, summary string, safe bool) bool
}

type AutoApprover struct {
	AutoApproveSafe bool
}

func (a AutoApprover) Approve(_ context.Context, _ string, _ string, safe bool) bool {
	return safe && a.AutoApproveSafe
}

type TerminalApprover struct {
	In              io.Reader
	Out             io.Writer
	AutoApproveSafe bool
}

func (t TerminalApprover) Approve(_ context.Context, toolName string, summary string, safe bool) bool {
	if safe && t.AutoApproveSafe {
		return true
	}
	if strings.Contains(strings.ToLower(summary), "denied") {
		fmt.Fprintf(t.Out, "\nDenied by policy for %s\n%s\n", toolName, summary)
		return false
	}
	fmt.Fprintf(t.Out, "\nApproval required for %s\n%s\nApprove? [y/N]: ", toolName, summary)
	reader := bufio.NewReader(t.In)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

type Hooks interface {
	OnToolCall(call llm.ToolCall, safe bool, summary string)
	OnToolResult(call llm.ToolCall, result tools.Result)
	OnFinalText(text string)
}

type Runner struct {
	cfg      Config
	client   *llm.Client
	registry *tools.Registry
	approver Approver
	audit    *security.AuditLog
	logger   *log.Logger

	auditCloser io.Closer
	logCloser   io.Closer
}

func NewRunner(cfg Config, approver Approver) (*Runner, error) {
	pathPolicy := security.NewPathPolicy(cfg.AllowedRoots)
	cmdPolicy := security.DefaultCommandPolicy()
	auditWriter, closer, err := newAuditWriter(cfg.AuditLogPath)
	if err != nil {
		return nil, err
	}
	logger, logCloser, err := newAppLogger(cfg)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	audit := security.NewAuditLog(auditWriter)
	registry := tools.NewRegistry(pathPolicy, cmdPolicy, audit, cfg.ToolOutputMaxBytes)

	client := llm.NewClient(cfg.BaseURL, cfg.APIKey, cfg.AccessToken, cfg.AccountID, cfg.Model, string(cfg.AuthMode))
	runner := &Runner{
		cfg:      cfg,
		client:   client,
		registry: registry,
		approver: approver,
		audit:    audit,
		logger:   logger,

		auditCloser: closer,
		logCloser:   logCloser,
	}
	runner.logger.Printf("runner initialized: %s", runnerLogSummary(cfg))
	return runner, nil
}

func (r *Runner) Close() error {
	var errs []error
	if r.logCloser != nil {
		if err := r.logCloser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.auditCloser != nil {
		if err := r.auditCloser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) Run(ctx context.Context, req Request) (Response, error) {
	return r.run(ctx, req, nil)
}

func (r *Runner) RunWithHooks(ctx context.Context, req Request, hooks Hooks) (Response, error) {
	return r.run(ctx, req, hooks)
}

func (r *Runner) run(ctx context.Context, req Request, hooks Hooks) (Response, error) {
	if strings.TrimSpace(req.Message) == "" {
		return Response{}, errors.New("message is required")
	}

	messages := []llm.Message{{Role: "system", Content: systemPrompt}}
	for _, turn := range req.History {
		role := strings.TrimSpace(turn.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		if strings.TrimSpace(turn.Content) == "" {
			continue
		}
		messages = append(messages, llm.Message{Role: role, Content: turn.Content})
	}
	messages = append(messages, llm.Message{Role: "user", Content: req.Message})

	toolRuns := make([]ToolRunInfo, 0)

	for step := 1; step <= r.cfg.MaxSteps; step++ {
		stepCtx, cancel := context.WithTimeout(ctx, r.cfg.StepTimeout)
		resp, err := r.client.Chat(stepCtx, messages, r.registry.Specs())
		cancel()
		if err != nil {
			return Response{}, err
		}

		messages = append(messages, resp.AssistantMessage())
		if len(resp.Message.ToolCalls) == 0 {
			if hooks != nil {
				hooks.OnFinalText(resp.Text())
			}
			return Response{
				Output:   resp.Text(),
				Steps:    step,
				Mode:     r.cfg.Mode,
				ToolRuns: toolRuns,
			}, nil
		}

		for _, call := range resp.Message.ToolCalls {
			safe, summary := r.registry.SafetyCheck(call)
			if hooks != nil {
				hooks.OnToolCall(call, safe, summary)
			}
			approved := r.approver.Approve(ctx, call.Function.Name, summary, safe)
			toolRuns = append(toolRuns, ToolRunInfo{
				Name:     call.Function.Name,
				Approved: approved,
				Status:   "pending",
			})

			var result tools.Result
			if approved {
				result = r.registry.Execute(ctx, call)
			} else {
				errMsg := "tool execution denied by approval policy"
				if strings.Contains(strings.ToLower(summary), "denied") {
					errMsg = summary
				}
				result = tools.Result{OK: false, Error: errMsg}
			}
			if hooks != nil {
				hooks.OnToolResult(call, result)
			}
			toolRuns[len(toolRuns)-1].Status = result.Status()

			payload, _ := json.Marshal(result)
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    string(payload),
			})
		}
	}

	return Response{}, fmt.Errorf("agent stopped after reaching max steps (%d)", r.cfg.MaxSteps)
}

func RunTerminal(ctx context.Context, cfg Config, in io.Reader, out io.Writer) error {
	runner, err := NewRunner(cfg, TerminalApprover{In: in, Out: out, AutoApproveSafe: true})
	if err != nil {
		return err
	}
	defer runner.Close()

	scanner := bufio.NewScanner(in)
	history := make([]HistoryTurn, 0, 16)
	fmt.Fprintln(out, "easyBot terminal mode. Type 'exit' to quit.")
	for {
		fmt.Fprint(out, "\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return nil
		}

		resp, err := runner.Run(ctx, Request{
			Message:  line,
			Approval: ApprovalOptions{AutoApproveSafe: true},
			History:  history,
		})
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
			continue
		}
		history = append(history, HistoryTurn{Role: "user", Content: line})
		history = append(history, HistoryTurn{Role: "assistant", Content: resp.Output})
		fmt.Fprintf(out, "%s\n", resp.Output)
	}
}

func newAuditWriter(path string) (io.Writer, io.Closer, error) {
	return newFileWriter(path)
}
