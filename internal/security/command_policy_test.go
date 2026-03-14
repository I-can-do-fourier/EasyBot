package security

import "testing"

func TestCommandPolicyDeniesDangerousCommand(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"rm", "-rf", "/tmp/x"})
	if decision.Safe || !decision.Denied {
		t.Fatal("expected rm to be denied")
	}
}

func TestCommandPolicyAllowsReadOnlyCommand(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"ls", "-la"})
	if !decision.Safe {
		t.Fatalf("expected ls to be safe, got: %s", decision.Summary)
	}
}

func TestCommandPolicyRequiresApprovalForUnknownCommand(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"fooctl", "status"})
	if decision.Safe || !decision.Denied {
		t.Fatalf("expected unknown command to be denied, got: %+v", decision)
	}
}

func TestCommandPolicyRequiresApprovalForCopy(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"cp", "a.txt", "b.txt"})
	if decision.Safe || decision.Denied {
		t.Fatalf("expected cp to require approval, got: %+v", decision)
	}
}

func TestCommandPolicyDeniesFindDelete(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"find", ".", "-delete"})
	if decision.Safe || !decision.Denied {
		t.Fatalf("expected find -delete to be denied, got: %+v", decision)
	}
}

func TestCommandPolicyAllowsReadOnlyGitStatus(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"git", "status"})
	if !decision.Safe || decision.Denied {
		t.Fatalf("expected git status to be allowed, got: %+v", decision)
	}
}

func TestCommandPolicyDeniesGitDelete(t *testing.T) {
	policy := DefaultCommandPolicy()
	decision := policy.Check([]string{"git", "rm", "a.txt"})
	if decision.Safe || !decision.Denied {
		t.Fatalf("expected git rm to be denied, got: %+v", decision)
	}
}
