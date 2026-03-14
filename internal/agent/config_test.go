package agent

import (
	"path/filepath"
	"testing"

	"easybot/internal/auth"
)

func TestLoadConfigFromEnvUsesSavedAPIKey(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")

	t.Setenv("EASYBOT_AUTH_FILE", authPath)
	if _, err := auth.Save(auth.File{APIKey: "sk-saved"}); err != nil {
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
