package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/I-can-do-fourier/EasyBot/internal/agent"
)

func TestParseArgsKeepsLoginSubcommand(t *testing.T) {
	cfg := agent.DefaultConfig()
	cfg.AllowedRoots = []string{t.TempDir()}

	runMode, _, _, err := parseArgs(cfg, []string{"login"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if runMode != "login" {
		t.Fatalf("runMode = %q, want %q", runMode, "login")
	}
}

func TestParseArgsUpdatesAppLogPath(t *testing.T) {
	cfg := agent.DefaultConfig()
	cfg.AllowedRoots = []string{t.TempDir()}

	_, _, parsed, err := parseArgs(cfg, []string{"--app-log", "logs/custom.log"})
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if parsed.AppLogPath != "logs/custom.log" {
		t.Fatalf("parsed.AppLogPath = %q, want %q", parsed.AppLogPath, "logs/custom.log")
	}
}

func TestUsageHidesHTTPAndACPModes(t *testing.T) {
	var buf bytes.Buffer
	fs, _ := newFlagSet("terminal", &buf)
	fs.Usage()
	got := buf.String()

	for _, hidden := range []string{"./easybot --http --listen :8080", "./easybot acp", "./easybot --acp", "-http", "-listen", "-acp"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("usage unexpectedly contains %q\n%s", hidden, got)
		}
	}

	for _, visible := range []string{"./easybot", "./easybot login", "-login", "-auth-mode"} {
		if !strings.Contains(got, visible) {
			t.Fatalf("usage missing %q\n%s", visible, got)
		}
	}
}

func TestUsageShowsBuiltInDefaultsInsteadOfResolvedConfig(t *testing.T) {
	var buf bytes.Buffer
	fs, _ := newFlagSet("terminal", &buf)
	fs.Usage()
	got := buf.String()

	for _, want := range []string{
		`default "https://api.openai.com/v1"`,
		`default "gpt-4.1-mini"`,
		`default "api_key"`,
		`default "easybot.log"`,
		`default "65536"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage missing built-in default %q\n%s", want, got)
		}
	}

	for _, unwanted := range []string{
		`default "false"`,
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("usage unexpectedly contains %q\n%s", unwanted, got)
		}
	}
}

func TestParseArgsKeepsResolvedConfigWhenFlagsAreUnset(t *testing.T) {
	cfg := agent.Config{
		BaseURL:            "https://example.com/v99",
		Model:              "custom-model",
		AuthMode:           agent.AuthModeCodex,
		AllowedRoots:       []string{"/tmp/custom-root"},
		MaxSteps:           99,
		StepTimeout:        99 * time.Second,
		Mode:               agent.ModeACP,
		AppLogPath:         "custom.log",
		AuditLogPath:       "/tmp/custom-audit.jsonl",
		ToolOutputMaxBytes: 99999,
	}

	runMode, _, parsed, err := parseArgs(cfg, nil)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if runMode != "acp" {
		t.Fatalf("runMode = %q, want %q", runMode, "acp")
	}
	if parsed.BaseURL != cfg.BaseURL {
		t.Fatalf("parsed.BaseURL = %q, want %q", parsed.BaseURL, cfg.BaseURL)
	}
	if parsed.Model != cfg.Model {
		t.Fatalf("parsed.Model = %q, want %q", parsed.Model, cfg.Model)
	}
	if parsed.AuthMode != cfg.AuthMode {
		t.Fatalf("parsed.AuthMode = %q, want %q", parsed.AuthMode, cfg.AuthMode)
	}
	if parsed.AppLogPath != cfg.AppLogPath {
		t.Fatalf("parsed.AppLogPath = %q, want %q", parsed.AppLogPath, cfg.AppLogPath)
	}
	if parsed.ToolOutputMaxBytes != cfg.ToolOutputMaxBytes {
		t.Fatalf("parsed.ToolOutputMaxBytes = %d, want %d", parsed.ToolOutputMaxBytes, cfg.ToolOutputMaxBytes)
	}
}
