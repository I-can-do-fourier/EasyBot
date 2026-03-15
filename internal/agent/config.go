package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/I-can-do-fourier/EasyBot/internal/auth"
)

type Mode string
type AuthMode string

const (
	ModeNonACP     Mode     = "non-acp"
	ModeACP        Mode     = "acp"
	AuthModeAPIKey AuthMode = "api_key"
	AuthModeCodex  AuthMode = "codex_oauth"
)

type Config struct {
	BaseURL            string
	APIKey             string
	AccessToken        string
	AccountID          string
	Model              string
	AuthMode           AuthMode
	AllowedRoots       []string
	MaxSteps           int
	StepTimeout        time.Duration
	Mode               Mode
	LogLevel           string
	AppLogPath         string
	AuditLogPath       string
	ToolOutputMaxBytes int
}

func LoadConfigFromEnv() (Config, error) {
	savedAuth, err := auth.Load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		savedAuth = auth.File{}
	}

	authMode := AuthMode(envDefault("EASYBOT_AUTH_MODE", fallbackString(savedAuth.AuthMode, string(AuthModeAPIKey))))
	baseURL := envDefault("EASYBOT_BASE_URL", fallbackString(savedAuth.BaseURL, defaultBaseURL(authMode)))
	model := envDefault("EASYBOT_MODEL", fallbackString(savedAuth.Model, "gpt-4.1-mini"))

	apiKey := strings.TrimSpace(os.Getenv("EASYBOT_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(savedAuth.APIKey)
	}
	accessToken := strings.TrimSpace(os.Getenv("EASYBOT_ACCESS_TOKEN"))
	if accessToken == "" {
		accessToken = strings.TrimSpace(savedAuth.AccessToken)
	}
	accountID := strings.TrimSpace(os.Getenv("EASYBOT_ACCOUNT_ID"))
	if accountID == "" {
		accountID = strings.TrimSpace(savedAuth.AccountID)
	}

	cfg := Config{
		BaseURL:            baseURL,
		APIKey:             apiKey,
		AccessToken:        accessToken,
		AccountID:          accountID,
		Model:              model,
		AuthMode:           authMode,
		AllowedRoots:       splitCSV(envDefault("EASYBOT_ALLOWED_ROOTS", defaultAllowedRoots())),
		MaxSteps:           envInt("EASYBOT_MAX_STEPS", 8),
		StepTimeout:        time.Duration(envInt("EASYBOT_STEP_TIMEOUT_SEC", 45)) * time.Second,
		Mode:               Mode(envDefault("EASYBOT_MODE", string(ModeNonACP))),
		LogLevel:           envDefault("EASYBOT_LOG_LEVEL", "info"),
		AppLogPath:         envDefault("EASYBOT_APP_LOG", "easybot.log"),
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
	if cfg.AuthMode != AuthModeAPIKey && cfg.AuthMode != AuthModeCodex {
		return Config{}, fmt.Errorf("EASYBOT_AUTH_MODE must be %q or %q", AuthModeAPIKey, AuthModeCodex)
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

func fallbackString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func defaultBaseURL(authMode AuthMode) string {
	if authMode == AuthModeCodex {
		return "https://chatgpt.com/backend-api/codex"
	}
	return "https://api.openai.com/v1"
}
