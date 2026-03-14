package main

import (
	"testing"
	"time"

	"easybot/internal/agent"
)

func TestParseArgsKeepsLoginSubcommand(t *testing.T) {
	cfg := agent.Config{
		AllowedRoots:       []string{t.TempDir()},
		MaxSteps:           8,
		StepTimeout:        45 * time.Second,
		ToolOutputMaxBytes: 65536,
	}

	runMode, _, _, err := parseArgs(cfg, []string{"login"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if runMode != "login" {
		t.Fatalf("runMode = %q, want %q", runMode, "login")
	}
}
