package tools

import (
	"os"
	"testing"

	"github.com/I-can-do-fourier/EasyBot/internal/security"
)

func TestWriteFileSafetySummaryExistingFileRequiresApproval(t *testing.T) {
	root := t.TempDir()
	path := root + "/notes.txt"
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewWriteFileTool(security.NewPathPolicy([]string{root}), security.NewAuditLog(nil))
	safe, summary := tool.SafetySummary(`{"path":"` + path + `","content":"new"}`)
	if safe {
		t.Fatal("expected existing file write to require approval")
	}
	if summary != "changing an existing file requires explicit human approval" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestWriteFileSafetySummaryNewFileRequiresApproval(t *testing.T) {
	root := t.TempDir()
	path := root + "/new.txt"

	tool := NewWriteFileTool(security.NewPathPolicy([]string{root}), security.NewAuditLog(nil))
	safe, summary := tool.SafetySummary(`{"path":"` + path + `","content":"new"}`)
	if safe {
		t.Fatal("expected new file write to require approval")
	}
	if summary != "creating a new file requires explicit human approval" {
		t.Fatalf("unexpected summary: %q", summary)
	}
}
