package acp

import "testing"

func TestFlattenPromptUsesMessageWhenPresent(t *testing.T) {
	got := flattenPrompt(sessionPromptParams{
		Message: "hello",
	}, "/tmp/work")
	if got != "hello" {
		t.Fatalf("unexpected flattened prompt: %q", got)
	}
}

func TestFlattenPromptJoinsBlocks(t *testing.T) {
	got := flattenPrompt(sessionPromptParams{
		Prompt: []promptBlock{
			{Type: "text", Text: "find the file"},
			{Type: "resource_link", URI: "file:///tmp/a.txt", Name: "a.txt"},
			{Type: "resource", Resource: &promptResource{URI: "file:///tmp/b.txt", Text: "notes"}},
		},
	}, "/tmp/work")
	if got == "" {
		t.Fatal("expected flattened prompt")
	}
	if want := "Current working directory: /tmp/work"; got[:len(want)] != want {
		t.Fatalf("missing cwd prefix: %q", got)
	}
}
