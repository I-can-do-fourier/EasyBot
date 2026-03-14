package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeNonACP Mode = "non-acp"
	ModeACP    Mode = "acp"
)

type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	AllowedRoots       []string
	MaxSteps           int
	StepTimeout        time.Duration
	Mode               Mode
	LogLevel           string
	AuditLogPath       string
	ToolOutputMaxBytes int
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:            envDefault("EASYBOT_BASE_URL", "https://api.openai.com/v1"),
		APIKey:             strings.TrimSpace(os.Getenv("EASYBOT_API_KEY")),
		Model:              envDefault("EASYBOT_MODEL", "gpt-4.1-mini"),
		AllowedRoots:       splitCSV(envDefault("EASYBOT_ALLOWED_ROOTS", defaultAllowedRoots())),
		MaxSteps:           envInt("EASYBOT_MAX_STEPS", 8),
		StepTimeout:        time.Duration(envInt("EASYBOT_STEP_TIMEOUT_SEC", 45)) * time.Second,
		Mode:               Mode(envDefault("EASYBOT_MODE", string(ModeNonACP))),
		LogLevel:           envDefault("EASYBOT_LOG_LEVEL", "info"),
		AuditLogPath:       envDefault("EASYBOT_AUDIT_LOG", filepath.Join(os.TempDir(), "easybot-audit.jsonl")),
		ToolOutputMaxBytes: envInt("EASYBOT_TOOL_OUTPUT_MAX_BYTES", 65536),
	}

	if cfg.MaxSteps <= 0 {
		return Config{}, fmt.Errorf("EASYBOT_MAX_STEPS must be > 0")
	}
	if cfg.StepTimeout <= 0 {
		return Config{}, fmt.Errorf("EASYBOT_STEP_TIMEOUT_SEC must be > 0")
	}
	if cfg.ToolOutputMaxBytes < 4096 {
		return Config{}, fmt.Errorf("EASYBOT_TOOL_OUTPUT_MAX_BYTES must be >= 4096")
	}
	if cfg.Mode != ModeNonACP && cfg.Mode != ModeACP {
		return Config{}, fmt.Errorf("EASYBOT_MODE must be %q or %q", ModeNonACP, ModeACP)
	}
	return cfg, nil
}

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultAllowedRoots() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".,/tmp"
	}
	return home + ",/tmp"
}
