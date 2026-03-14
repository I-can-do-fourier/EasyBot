package llm

import (
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
	Model string           `json:"model"`
	Input []map[string]any `json:"input"`
	Tools []responsesTool  `json:"tools,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
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

	body, err := json.Marshal(responsesRequest{
		Model: c.model,
		Input: buildResponsesInput(messages),
		Tools: buildResponsesTools(tools),
	})
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

	var decoded responsesResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Response{}, err
	}
	if decoded.Error != nil {
		return Response{}, fmt.Errorf("llm API error: %s", decoded.Error.Message)
	}

	textParts := make([]string, 0, len(decoded.Output))
	toolCalls := make([]ToolCall, 0)
	for _, item := range decoded.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Text != "" {
					textParts = append(textParts, content.Text)
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, ToolCall{
				ID:   firstNonEmpty(item.CallID, item.ID),
				Type: "function",
				Function: ToolCallFunction{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}

	return Response{
		Message: Message{
			Role:      "assistant",
			Content:   strings.Join(textParts, "\n"),
			ToolCalls: toolCalls,
		},
	}, nil
}

func buildResponsesInput(messages []Message) []map[string]any {
	input := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
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
			input = append(input, map[string]any{
				"role":    msg.Role,
				"content": msg.Content,
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
	return input
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
