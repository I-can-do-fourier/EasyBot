package agent

import (
	"path/filepath"
	"testing"

	"github.com/I-can-do-fourier/EasyBot/internal/auth"
)

func TestLoadConfigFromEnvUsesSavedAPIKey(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")

	t.Setenv("EASYBOT_AUTH_FILE", authPath)
	if _, err := auth.Save(auth.File{
		APIKey:   "sk-saved",
		BaseURL:  "https://api.example.test/v1",
		Model:    "saved-model",
		AuthMode: string(AuthModeAPIKey),
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	t.Setenv("EASYBOT_API_KEY", "")
	t.Setenv("EASYBOT_ALLOWED_ROOTS", root)
	t.Setenv("EASYBOT_BASE_URL", "")
	t.Setenv("EASYBOT_MODEL", "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.APIKey != "sk-saved" {
		t.Fatalf("cfg.APIKey = %q, want %q", cfg.APIKey, "sk-saved")
	}
	if cfg.BaseURL != "https://api.example.test/v1" {
		t.Fatalf("cfg.BaseURL = %q, want saved base URL", cfg.BaseURL)
	}
	if cfg.Model != "saved-model" {
		t.Fatalf("cfg.Model = %q, want saved model", cfg.Model)
	}
	if cfg.AuthMode != AuthModeAPIKey {
		t.Fatalf("cfg.AuthMode = %q, want %q", cfg.AuthMode, AuthModeAPIKey)
	}
}

func TestLoadConfigFromEnvPrefersEnvAPIKey(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")

	t.Setenv("EASYBOT_AUTH_FILE", authPath)
	if _, err := auth.Save(auth.File{APIKey: "sk-saved"}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	t.Setenv("EASYBOT_API_KEY", "sk-env")
	t.Setenv("EASYBOT_ALLOWED_ROOTS", root)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.APIKey != "sk-env" {
		t.Fatalf("cfg.APIKey = %q, want %q", cfg.APIKey, "sk-env")
	}
}

func TestLoadConfigFromEnvUsesSavedCodexOAuthSettings(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")

	t.Setenv("EASYBOT_AUTH_FILE", authPath)
	if _, err := auth.Save(auth.File{
		AccessToken: "oauth-token",
		AccountID:   "acct-123",
		BaseURL:     "https://chatgpt.com/backend-api/codex",
		Model:       "gpt-5-codex",
		AuthMode:    string(AuthModeCodex),
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	t.Setenv("EASYBOT_ACCESS_TOKEN", "")
	t.Setenv("EASYBOT_ACCOUNT_ID", "")
	t.Setenv("EASYBOT_ALLOWED_ROOTS", root)
	t.Setenv("EASYBOT_AUTH_MODE", "")
	t.Setenv("EASYBOT_BASE_URL", "")
	t.Setenv("EASYBOT_MODEL", "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.AuthMode != AuthModeCodex {
		t.Fatalf("cfg.AuthMode = %q, want %q", cfg.AuthMode, AuthModeCodex)
	}
	if cfg.AccessToken != "oauth-token" {
		t.Fatalf("cfg.AccessToken = %q, want %q", cfg.AccessToken, "oauth-token")
	}
	if cfg.AccountID != "acct-123" {
		t.Fatalf("cfg.AccountID = %q, want %q", cfg.AccountID, "acct-123")
	}
	if cfg.BaseURL != "https://chatgpt.com/backend-api/codex" {
		t.Fatalf("cfg.BaseURL = %q, want saved Codex base URL", cfg.BaseURL)
	}
	if cfg.Model != "gpt-5-codex" {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, "gpt-5-codex")
	}
}

func TestLoadConfigFromEnvUsesAppLogOverride(t *testing.T) {
	root := t.TempDir()
	wantLogPath := filepath.Join(root, "logs", "easybot.log")

	t.Setenv("EASYBOT_ALLOWED_ROOTS", root)
	t.Setenv("EASYBOT_APP_LOG", wantLogPath)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.AppLogPath != wantLogPath {
		t.Fatalf("cfg.AppLogPath = %q, want %q", cfg.AppLogPath, wantLogPath)
	}
}
