package acp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/I-can-do-fourier/EasyBot/internal/agent"
	"github.com/I-can-do-fourier/EasyBot/internal/llm"
	"github.com/I-can-do-fourier/EasyBot/internal/tools"
)

type Server struct {
	cfg       agent.Config
	out       io.Writer
	writeMu   sync.Mutex
	sessionMu sync.Mutex
	sessions  map[string]*session
}

type session struct {
	ID        string
	CWD       string
	Title     string
	UpdatedAt time.Time

	mu      sync.Mutex
	history []agent.HistoryTurn
	cancel  context.CancelFunc
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

type sessionNewParams struct {
	CWD   string `json:"cwd,omitempty"`
	Title string `json:"title,omitempty"`
}

type sessionListResult struct {
	Sessions []sessionInfo `json:"sessions"`
}

type sessionInfo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type sessionPromptParams struct {
	SessionID string        `json:"sessionId"`
	Message   string        `json:"message,omitempty"`
	Prompt    []promptBlock `json:"prompt,omitempty"`
}

type promptBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Name     string          `json:"name,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	Resource *promptResource `json:"resource,omitempty"`
}

type promptResource struct {
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

type sessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

func Serve(ctx context.Context, cfg agent.Config, in io.Reader, out io.Writer) error {
	server := &Server{
		cfg:      cfg,
		out:      out,
		sessions: make(map[string]*session),
	}

	dec := json.NewDecoder(bufio.NewReader(in))
	for {
		var msg rpcMessage
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(msg.Method) == "" {
			continue
		}
		go server.handle(ctx, msg)
	}
}

func (s *Server) handle(ctx context.Context, msg rpcMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "session/new":
		s.handleSessionNew(msg)
	case "session/list":
		s.handleSessionList(msg)
	case "session/prompt":
		s.handleSessionPrompt(ctx, msg)
	case "session/cancel":
		s.handleSessionCancel(msg)
	default:
		s.replyError(msg.ID, -32601, fmt.Sprintf("method not found: %s", msg.Method))
	}
}

func (s *Server) handleInitialize(msg rpcMessage) {
	var params initializeParams
	_ = json.Unmarshal(msg.Params, &params)
	version := strings.TrimSpace(params.ProtocolVersion)
	if version == "" {
		version = "0.1"
	}
	s.reply(msg.ID, map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"session/new":    true,
			"session/list":   true,
			"session/prompt": true,
			"session/cancel": true,
			"streaming":      true,
		},
		"agent": map[string]any{
			"name":    "easyBot",
			"version": "0.1.0",
		},
	})
}

func (s *Server) handleSessionNew(msg rpcMessage) {
	var params sessionNewParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.replyError(msg.ID, -32602, err.Error())
		return
	}

	id, err := newSessionID()
	if err != nil {
		s.replyError(msg.ID, -32000, err.Error())
		return
	}
	now := time.Now().UTC()
	sess := &session{
		ID:        id,
		CWD:       strings.TrimSpace(params.CWD),
		Title:     strings.TrimSpace(params.Title),
		UpdatedAt: now,
	}

	s.sessionMu.Lock()
	s.sessions[id] = sess
	s.sessionMu.Unlock()

	s.reply(msg.ID, map[string]any{
		"sessionId": id,
		"session": sessionInfo{
			ID:        id,
			Title:     sess.Title,
			CWD:       sess.CWD,
			UpdatedAt: sess.UpdatedAt,
		},
	})
}

func (s *Server) handleSessionList(msg rpcMessage) {
	s.sessionMu.Lock()
	list := make([]sessionInfo, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, sessionInfo{
			ID:        sess.ID,
			Title:     sess.Title,
			CWD:       sess.CWD,
			UpdatedAt: sess.UpdatedAt,
		})
	}
	s.sessionMu.Unlock()

	s.reply(msg.ID, sessionListResult{Sessions: list})
}

func (s *Server) handleSessionPrompt(ctx context.Context, msg rpcMessage) {
	var params sessionPromptParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.replyError(msg.ID, -32602, err.Error())
		return
	}
	if strings.TrimSpace(params.SessionID) == "" {
		s.replyError(msg.ID, -32602, "sessionId is required")
		return
	}

	sess := s.getSession(params.SessionID)
	if sess == nil {
		s.replyError(msg.ID, -32001, "session not found")
		return
	}

	prompt := flattenPrompt(params, sess.CWD)
	if strings.TrimSpace(prompt) == "" {
		s.replyError(msg.ID, -32602, "prompt is empty")
		return
	}

	history := sess.snapshotHistory()
	runCtx, cancel := context.WithCancel(ctx)
	sess.setCancel(cancel)
	defer func() {
		cancel()
		sess.clearCancel()
	}()

	runner, err := agent.NewRunner(s.cfg, agent.AutoApprover{AutoApproveSafe: true})
	if err != nil {
		s.replyError(msg.ID, -32000, err.Error())
		return
	}
	defer runner.Close()

	resp, err := runner.RunWithHooks(runCtx, agent.Request{
		Message:  prompt,
		Approval: agent.ApprovalOptions{AutoApproveSafe: true},
		History:  history,
	}, acpHooks{server: s, sessionID: sess.ID})
	if err != nil {
		if errors.Is(runCtx.Err(), context.Canceled) {
			s.reply(msg.ID, map[string]any{
				"stopReason": "cancelled",
				"output":     []any{},
			})
			return
		}
		s.replyError(msg.ID, -32000, err.Error())
		return
	}

	sess.appendHistory(agent.HistoryTurn{Role: "user", Content: prompt})
	sess.appendHistory(agent.HistoryTurn{Role: "assistant", Content: resp.Output})
	s.reply(msg.ID, map[string]any{
		"stopReason": "end_turn",
		"output": []map[string]any{
			{"type": "text", "text": resp.Output},
		},
	})
}

func (s *Server) handleSessionCancel(msg rpcMessage) {
	var params sessionCancelParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.replyError(msg.ID, -32602, err.Error())
		return
	}
	sess := s.getSession(params.SessionID)
	if sess == nil {
		s.replyError(msg.ID, -32001, "session not found")
		return
	}
	sess.mu.Lock()
	cancel := sess.cancel
	sess.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.reply(msg.ID, map[string]any{"ok": true})
}

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	s.write(rpcMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) replyError(id json.RawMessage, code int, message string) {
	if len(id) == 0 {
		return
	}
	s.write(rpcMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
}

func (s *Server) notify(method string, params any) {
	s.write(rpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustRaw(params),
	})
}

func (s *Server) write(msg rpcMessage) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = json.NewEncoder(s.out).Encode(msg)
}

func (s *Server) getSession(id string) *session {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.sessions[id]
}

func (sess *session) snapshotHistory() []agent.HistoryTurn {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	out := make([]agent.HistoryTurn, len(sess.history))
	copy(out, sess.history)
	return out
}

func (sess *session) appendHistory(turn agent.HistoryTurn) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.history = append(sess.history, turn)
	sess.UpdatedAt = time.Now().UTC()
}

func (sess *session) setCancel(cancel context.CancelFunc) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.cancel = cancel
}

func (sess *session) clearCancel() {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.cancel = nil
}

type acpHooks struct {
	server    *Server
	sessionID string
}

func (h acpHooks) OnToolCall(call llm.ToolCall, safe bool, summary string) {
	h.server.notify("session/update", map[string]any{
		"sessionId": h.sessionID,
		"update": map[string]any{
			"type":       "tool_call",
			"toolCallId": call.ID,
			"name":       call.Function.Name,
			"safe":       safe,
			"summary":    summary,
			"arguments":  mustObject(call.Function.Arguments),
		},
	})
}

func (h acpHooks) OnToolResult(call llm.ToolCall, result tools.Result) {
	h.server.notify("session/update", map[string]any{
		"sessionId": h.sessionID,
		"update": map[string]any{
			"type":       "tool_call_update",
			"toolCallId": call.ID,
			"status":     result.Status(),
			"result":     result,
		},
	})
}

func (h acpHooks) OnFinalText(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	h.server.notify("session/update", map[string]any{
		"sessionId": h.sessionID,
		"update": map[string]any{
			"type": "agent_message_chunk",
			"text": text,
		},
	})
}

func flattenPrompt(params sessionPromptParams, cwd string) string {
	if strings.TrimSpace(params.Message) != "" {
		return strings.TrimSpace(params.Message)
	}
	parts := make([]string, 0, len(params.Prompt)+1)
	if strings.TrimSpace(cwd) != "" {
		parts = append(parts, "Current working directory: "+cwd)
	}
	for _, block := range params.Prompt {
		switch strings.ToLower(strings.TrimSpace(block.Type)) {
		case "text":
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		case "resource":
			if block.Resource != nil {
				if text := strings.TrimSpace(block.Resource.Text); text != "" {
					label := strings.TrimSpace(block.Resource.URI)
					if label == "" {
						label = "embedded resource"
					}
					parts = append(parts, label+":\n"+text)
				} else if uri := strings.TrimSpace(block.Resource.URI); uri != "" {
					parts = append(parts, "Resource: "+uri)
				}
			}
		case "resource_link", "resourcelink", "resource-link":
			label := strings.TrimSpace(block.Name)
			uri := strings.TrimSpace(block.URI)
			switch {
			case label != "" && uri != "":
				parts = append(parts, fmt.Sprintf("Resource link: %s (%s)", uri, label))
			case uri != "":
				parts = append(parts, "Resource link: "+uri)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func mustRaw(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func mustObject(raw string) any {
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{"raw": raw}
	}
	return out
}

func newSessionID() (string, error) {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
