package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("EASYBOT_AUTH_FILE", authPath)

	want := File{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		IDToken:      "id-token",
		TokenType:    "Bearer",
		AccountID:    "acct_123",
		ExpiresAt:    time.Unix(1_700_000_000, 0).UTC(),
		APIKey:       "sk-test",
		BaseURL:      "https://chatgpt.com/backend-api/codex",
		Model:        "gpt-5-codex",
		AuthMode:     "codex_oauth",
	}

	path, err := Save(want)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if path != authPath {
		t.Fatalf("Save path = %q, want %q", path, authPath)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load returned %+v, want %+v", got, want)
	}

	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth file permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("EASYBOT_AUTH_FILE", filepath.Join(t.TempDir(), "missing.json"))

	_, err := Load()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load error = %v, want os.ErrNotExist", err)
	}
}
