package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL     string
	apiKey      string
	accessToken string
	accountID   string
	model       string
	authMode    string
	httpClient  *http.Client
}

func NewClient(baseURL, apiKey, accessToken, accountID, model, authMode string) *Client {
	authMode = strings.TrimSpace(authMode)
	if authMode == "" {
		authMode = "api_key"
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		accessToken: accessToken,
		accountID:   accountID,
		model:       model,
		authMode:    authMode,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolSpec struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatRequest struct {
	Model    string     `json:"model"`
	Messages []Message  `json:"messages"`
	Tools    []ToolSpec `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type Response struct {
	Message Message
}

func (r Response) AssistantMessage() Message { return r.Message }
func (r Response) Text() string              { return r.Message.Content }
func (r Response) ToolCalls() []ToolCall     { return r.Message.ToolCalls }

func (c *Client) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Response, error) {
	if c.authMode == "codex_oauth" {
		return c.responsesChat(ctx, messages, tools)
	}
	return c.chatCompletions(ctx, messages, tools)
}

func (c *Client) chatCompletions(ctx context.Context, messages []Message, tools []ToolSpec) (Response, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return Response{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "easybot/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("llm API error: %s", strings.TrimSpace(string(raw)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Response{}, err
	}
	if decoded.Error != nil {
		return Response{}, fmt.Errorf("llm API error: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("llm API returned no choices")
	}
	return Response{Message: decoded.Choices[0].Message}, nil
}

type responsesRequest struct {
	Model             string           `json:"model"`
	Store             bool             `json:"store"`
	Stream            bool             `json:"stream"`
	Instructions      string           `json:"instructions,omitempty"`
	Input             []map[string]any `json:"input"`
	Text              map[string]any   `json:"text,omitempty"`
	Include           []string         `json:"include,omitempty"`
	ToolChoice        string           `json:"tool_choice,omitempty"`
	ParallelToolCalls bool             `json:"parallel_tool_calls,omitempty"`
	Tools             []responsesTool  `json:"tools,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      any            `json:"strict"`
}

type responsesResponse struct {
	ID     string                `json:"id"`
	Output []responsesOutputItem `json:"output"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type responsesOutputItem struct {
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Content   []responsesContentItem `json:"content,omitempty"`
}

type responsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func (c *Client) responsesChat(ctx context.Context, messages []Message, tools []ToolSpec) (Response, error) {
	if strings.TrimSpace(c.accessToken) == "" {
		return Response{}, fmt.Errorf("codex OAuth mode requires an access token")
	}

	instructions, input := buildResponsesInput(messages)
	responseTools := buildResponsesTools(tools)
	request := responsesRequest{
		Model:        c.model,
		Store:        false,
		Stream:       true,
		Instructions: instructions,
		Input:        input,
		Text:         map[string]any{"verbosity": "medium"},
		Include:      []string{"reasoning.encrypted_content"},
		Tools:        responseTools,
	}
	if len(responseTools) > 0 {
		request.ToolChoice = "auto"
		request.ParallelToolCalls = true
	}

	body, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if strings.TrimSpace(c.accountID) != "" {
		req.Header.Set("ChatGPT-Account-ID", c.accountID)
	}
	req.Header.Set("Originator", "pi")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pi (easybot/0.1)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return Response{}, err
		}
		return Response{}, fmt.Errorf("llm API error: %s", strings.TrimSpace(string(raw)))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return parseResponsesSSE(resp.Body)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	if looksLikeSSE(raw) {
		return parseResponsesSSE(bytes.NewReader(raw))
	}
	return parseResponsesJSON(raw)
}

func parseResponsesJSON(raw []byte) (Response, error) {
	if looksLikeSSE(raw) {
		return parseResponsesSSE(bytes.NewReader(raw))
	}

	var decoded responsesResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Response{}, err
	}
	if decoded.Error != nil {
		return Response{}, fmt.Errorf("llm API error: %s", decoded.Error.Message)
	}
	return responseFromOutput(decoded.Output), nil
}

func parseResponsesSSE(body io.Reader) (Response, error) {
	reader := bufio.NewReader(body)
	var eventType string
	var dataLines []string
	accumulator := responseAccumulator{}

	flush := func() error {
		if len(dataLines) == 0 {
			eventType = ""
			return nil
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		eventType = strings.TrimSpace(eventType)
		dataLines = nil
		if payload == "" {
			eventType = ""
			return nil
		}
		if payload == "[DONE]" {
			eventType = ""
			return io.EOF
		}
		if err := accumulator.consume(eventType, payload); err != nil {
			return err
		}
		eventType = ""
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return Response{}, err
		}

		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if flushErr := flush(); flushErr != nil {
				if flushErr == io.EOF {
					return accumulator.response(), nil
				}
				return Response{}, flushErr
			}
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}

		if err == io.EOF {
			if flushErr := flush(); flushErr != nil && flushErr != io.EOF {
				return Response{}, flushErr
			}
			return accumulator.response(), nil
		}
	}
}

type responseAccumulator struct {
	textParts         []string
	toolCalls         map[string]ToolCall
	currentBlockKind  string
	currentToolCallID string
}

func (a *responseAccumulator) consume(eventType, payload string) error {
	var envelope struct {
		Type     string               `json:"type"`
		Delta    string               `json:"delta"`
		Item     *responsesOutputItem `json:"item"`
		Response *responsesResponse   `json:"response"`
		Message  string               `json:"message"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return err
	}

	kind := firstNonEmpty(envelope.Type, eventType)
	switch kind {
	case "response.output_item.added":
		if envelope.Item != nil {
			a.beginItem(*envelope.Item)
		}
	case "response.output_text.delta":
		if envelope.Delta != "" {
			a.appendTextDelta(envelope.Delta)
		}
	case "response.reasoning_summary_text.delta":
		// Reasoning blocks are intentionally ignored for now because the agent
		// only needs final text plus tool calls.
	case "response.function_call_arguments.delta":
		if envelope.Delta != "" {
			a.appendToolArgumentsDelta(envelope.Delta)
		}
	case "response.output_item.done":
		if envelope.Item != nil {
			a.consumeOutputItem(*envelope.Item)
		}
	case "response.completed":
		if envelope.Response != nil && len(a.textParts) == 0 && len(a.toolCalls) == 0 {
			for _, item := range envelope.Response.Output {
				a.consumeOutputItem(item)
			}
		}
	case "response.failed", "error":
		if envelope.Error != nil && envelope.Error.Message != "" {
			return fmt.Errorf("llm API error: %s", envelope.Error.Message)
		}
		if envelope.Message != "" {
			return fmt.Errorf("llm API error: %s", envelope.Message)
		}
	}
	return nil
}

func (a *responseAccumulator) beginItem(item responsesOutputItem) {
	switch item.Type {
	case "message":
		a.textParts = append(a.textParts, "")
		a.currentBlockKind = "text"
	case "function_call":
		if a.toolCalls == nil {
			a.toolCalls = make(map[string]ToolCall)
		}
		id := firstNonEmpty(item.CallID, item.ID)
		call := a.toolCalls[id]
		call.ID = id
		call.Type = "function"
		call.Function.Name = item.Name
		a.toolCalls[id] = call
		a.currentBlockKind = "tool_call"
		a.currentToolCallID = id
	default:
		a.currentBlockKind = ""
		a.currentToolCallID = ""
	}
}

func (a *responseAccumulator) appendTextDelta(delta string) {
	if len(a.textParts) == 0 {
		a.textParts = append(a.textParts, "")
	}
	a.textParts[len(a.textParts)-1] += delta
	a.currentBlockKind = "text"
}

func (a *responseAccumulator) appendToolArgumentsDelta(delta string) {
	if a.currentToolCallID == "" {
		return
	}
	call := a.toolCalls[a.currentToolCallID]
	call.Function.Arguments += delta
	a.toolCalls[a.currentToolCallID] = call
}

func (a *responseAccumulator) consumeOutputItem(item responsesOutputItem) {
	if a.toolCalls == nil {
		a.toolCalls = make(map[string]ToolCall)
	}
	switch item.Type {
	case "message":
		text := ""
		for _, content := range item.Content {
			if content.Text != "" {
				text += content.Text
			}
		}
		if text != "" {
			if a.currentBlockKind == "text" && len(a.textParts) > 0 && a.textParts[len(a.textParts)-1] == "" {
				a.textParts[len(a.textParts)-1] = text
			} else {
				a.textParts = append(a.textParts, text)
			}
		}
		a.currentBlockKind = ""
	case "function_call":
		id := firstNonEmpty(item.CallID, item.ID)
		call := a.toolCalls[id]
		call.ID = id
		call.Type = "function"
		if call.Function.Name == "" {
			call.Function.Name = item.Name
		} else if item.Name != "" {
			call.Function.Name = item.Name
		}
		if item.Arguments != "" {
			call.Function.Arguments = item.Arguments
		}
		a.toolCalls[id] = ToolCall{
			ID:       call.ID,
			Type:     call.Type,
			Function: call.Function,
		}
		a.currentBlockKind = ""
		a.currentToolCallID = ""
	}
}

func (a *responseAccumulator) response() Response {
	toolCalls := make([]ToolCall, 0, len(a.toolCalls))
	for _, call := range a.toolCalls {
		toolCalls = append(toolCalls, call)
	}
	return Response{
		Message: Message{
			Role:      "assistant",
			Content:   strings.Join(a.textParts, ""),
			ToolCalls: toolCalls,
		},
	}
}

func responseFromOutput(output []responsesOutputItem) Response {
	accumulator := responseAccumulator{}
	for _, item := range output {
		accumulator.consumeOutputItem(item)
	}
	return accumulator.response()
}

func buildResponsesInput(messages []Message) (string, []map[string]any) {
	instructions := make([]string, 0, 1)
	input := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			if strings.TrimSpace(msg.Content) != "" {
				instructions = append(instructions, msg.Content)
			}
			continue
		}

		if msg.Role == "tool" {
			item := map[string]any{
				"type":   "function_call_output",
				"output": msg.Content,
			}
			if msg.ToolCallID != "" {
				item["call_id"] = msg.ToolCallID
			}
			input = append(input, item)
			continue
		}

		if strings.TrimSpace(msg.Content) != "" {
			contentType := "input_text"
			if msg.Role == "assistant" {
				contentType = "output_text"
			}
			input = append(input, map[string]any{
				"type": "message",
				"role": msg.Role,
				"content": []map[string]any{{
					"type": contentType,
					"text": msg.Content,
				}},
			})
		}
		for _, call := range msg.ToolCalls {
			input = append(input, map[string]any{
				"type":      "function_call",
				"call_id":   call.ID,
				"name":      call.Function.Name,
				"arguments": call.Function.Arguments,
			})
		}
	}
	return strings.Join(instructions, "\n\n"), input
}

func buildResponsesTools(tools []ToolSpec) []responsesTool {
	out := make([]responsesTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		out = append(out, responsesTool{
			Type:        "function",
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
			Strict:      nil,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func looksLikeSSE(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:"))
}
