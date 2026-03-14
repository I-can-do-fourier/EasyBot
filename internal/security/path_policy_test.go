package security

import (
	"path/filepath"
	"testing"
)

func TestPathPolicyResolveInsideRoot(t *testing.T) {
	root := t.TempDir()
	policy := NewPathPolicy([]string{root})

	got, err := policy.Resolve(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got != filepath.Join(root, "notes.txt") {
		t.Fatalf("unexpected resolved path: %s", got)
	}
}

func TestPathPolicyRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	policy := NewPathPolicy([]string{root})

	if _, err := policy.Resolve("/tmp/outside.txt"); err == nil {
		t.Fatal("expected outside path to be rejected")
	}
}
