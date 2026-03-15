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

func DefaultConfig() Config {
	authMode := AuthModeAPIKey
	return Config{
		BaseURL:            defaultBaseURL(authMode),
		Model:              "gpt-4.1-mini",
		AuthMode:           authMode,
		AllowedRoots:       splitCSV(defaultAllowedRoots()),
		MaxSteps:           8,
		StepTimeout:        45 * time.Second,
		Mode:               ModeNonACP,
		LogLevel:           "info",
		AppLogPath:         "easybot.log",
		AuditLogPath:       filepath.Join(os.TempDir(), "easybot-audit.jsonl"),
		ToolOutputMaxBytes: 65536,
	}
}

func LoadConfigFromEnv() (Config, error) {
	defaultCfg := DefaultConfig()

	savedAuth, err := auth.Load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		savedAuth = auth.File{}
	}

	authMode := AuthMode(envDefault("EASYBOT_AUTH_MODE", fallbackString(savedAuth.AuthMode, string(defaultCfg.AuthMode))))
	baseURL := envDefault("EASYBOT_BASE_URL", fallbackString(savedAuth.BaseURL, defaultBaseURL(authMode)))
	model := envDefault("EASYBOT_MODEL", fallbackString(savedAuth.Model, defaultCfg.Model))

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

	cfg := defaultCfg
	cfg.BaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.AccessToken = accessToken
	cfg.AccountID = accountID
	cfg.Model = model
	cfg.AuthMode = authMode
	cfg.AllowedRoots = splitCSV(envDefault("EASYBOT_ALLOWED_ROOTS", strings.Join(defaultCfg.AllowedRoots, ",")))
	cfg.MaxSteps = envInt("EASYBOT_MAX_STEPS", defaultCfg.MaxSteps)
	cfg.StepTimeout = time.Duration(envInt("EASYBOT_STEP_TIMEOUT_SEC", int(defaultCfg.StepTimeout/time.Second))) * time.Second
	cfg.Mode = Mode(envDefault("EASYBOT_MODE", string(defaultCfg.Mode)))
	cfg.LogLevel = envDefault("EASYBOT_LOG_LEVEL", defaultCfg.LogLevel)
	cfg.AppLogPath = envDefault("EASYBOT_APP_LOG", defaultCfg.AppLogPath)
	cfg.AuditLogPath = envDefault("EASYBOT_AUDIT_LOG", defaultCfg.AuditLogPath)
	cfg.ToolOutputMaxBytes = envInt("EASYBOT_TOOL_OUTPUT_MAX_BYTES", defaultCfg.ToolOutputMaxBytes)

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
