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
		AppLogPath:         "easybot.log",
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

func TestParseArgsUpdatesAppLogPath(t *testing.T) {
	cfg := agent.Config{
		AllowedRoots:       []string{t.TempDir()},
		MaxSteps:           8,
		StepTimeout:        45 * time.Second,
		AppLogPath:         "easybot.log",
		ToolOutputMaxBytes: 65536,
	}

	_, _, parsed, err := parseArgs(cfg, []string{"--app-log", "logs/custom.log"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if parsed.AppLogPath != "logs/custom.log" {
		t.Fatalf("parsed.AppLogPath = %q, want %q", parsed.AppLogPath, "logs/custom.log")
	}
}
