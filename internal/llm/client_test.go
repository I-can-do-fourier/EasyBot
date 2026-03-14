package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestChatCompletionsModeUsesAPIKeyAndChatCompletionsPath(t *testing.T) {
	client := NewClient("https://api.openai.com/v1", "sk-test", "", "", "gpt-4.1-mini", "api_key")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://api.openai.com/v1/chat/completions" {
				t.Fatalf("request URL = %q", r.URL.String())
			}
			if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
				t.Fatalf("Authorization = %q", got)
			}
			return jsonResponse(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Text() != "ok" {
		t.Fatalf("resp.Text = %q, want %q", resp.Text(), "ok")
	}
}

func TestCodexOAuthModeUsesResponsesPathAndAccessToken(t *testing.T) {
	client := NewClient("https://chatgpt.com/backend-api/codex", "", "oauth-token", "acct-123", "gpt-5-codex", "codex_oauth")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" {
				t.Fatalf("request URL = %q", r.URL.String())
			}
			if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
				t.Fatalf("Authorization = %q", got)
			}
			if got := r.Header.Get("ChatGPT-Account-ID"); got != "acct-123" {
				t.Fatalf("ChatGPT-Account-ID = %q", got)
			}
			if got := r.Header.Get("Originator"); got != "pi" {
				t.Fatalf("Originator = %q", got)
			}
			if got := r.Header.Get("OpenAI-Beta"); got != "responses=experimental" {
				t.Fatalf("OpenAI-Beta = %q", got)
			}
			if got := r.Header.Get("Accept"); got != "text/event-stream" {
				t.Fatalf("Accept = %q", got)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			if payload["model"] != "gpt-5-codex" {
				t.Fatalf("model = %#v", payload["model"])
			}
			if payload["store"] != false {
				t.Fatalf("store = %#v", payload["store"])
			}
			if payload["stream"] != true {
				t.Fatalf("stream = %#v", payload["stream"])
			}
			if payload["instructions"] != "system prompt" {
				t.Fatalf("instructions = %#v", payload["instructions"])
			}
			if payload["tool_choice"] != "auto" {
				t.Fatalf("tool_choice = %#v", payload["tool_choice"])
			}

			return sseResponse(strings.Join([]string{
				`event: response.output_item.added`,
				`data: {"type":"response.output_item.added","item":{"type":"message","id":"msg_123"}}`,
				``,
				`event: response.output_text.delta`,
				`data: {"type":"response.output_text.delta","delta":"thinking"}`,
				``,
				`event: response.output_item.done`,
				`data: {"type":"response.output_item.done","item":{"type":"message","id":"msg_123"}}`,
				``,
				`event: response.output_item.added`,
				`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_123","call_id":"call_123","name":"search_files"}}`,
				``,
				`event: response.function_call_arguments.delta`,
				`data: {"type":"response.function_call_arguments.delta","delta":"{\"pattern\":\"resume\"}"}`,
				``,
				`event: response.output_item.done`,
				`data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_123","call_id":"call_123","name":"search_files"}}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n")), nil
		}),
	}

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "find resume"},
	}, []ToolSpec{{
		Type: "function",
		Function: ToolFunction{
			Name:        "search_files",
			Description: "Search files",
			Parameters:  map[string]any{"type": "object"},
		},
	}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if !strings.Contains(resp.Text(), "thinking") {
		t.Fatalf("resp.Text = %q", resp.Text())
	}
	if len(resp.ToolCalls()) != 1 {
		t.Fatalf("len(resp.ToolCalls()) = %d, want 1", len(resp.ToolCalls()))
	}
	if resp.ToolCalls()[0].ID != "call_123" {
		t.Fatalf("tool call ID = %q", resp.ToolCalls()[0].ID)
	}
	if resp.ToolCalls()[0].Function.Arguments != `{"pattern":"resume"}` {
		t.Fatalf("tool call arguments = %q", resp.ToolCalls()[0].Function.Arguments)
	}
}

func TestCodexOAuthModeFallsBackToSSEWhenContentTypeIsWrong(t *testing.T) {
	client := NewClient("https://chatgpt.com/backend-api/codex", "", "oauth-token", "acct-123", "gpt-5-codex", "codex_oauth")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(strings.Join([]string{
				`event: response.output_item.added`,
				`data: {"type":"response.output_item.added","item":{"type":"message","id":"msg_123"}}`,
				``,
				`event: response.output_text.delta`,
				`data: {"type":"response.output_text.delta","delta":"hello from sse"}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n")), nil
		}),
	}

	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Text() != "hello from sse" {
		t.Fatalf("resp.Text = %q", resp.Text())
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func sseResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
