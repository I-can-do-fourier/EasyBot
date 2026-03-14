package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type File struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	APIKey       string    `json:"api_key,omitempty"`
	BaseURL      string    `json:"base_url,omitempty"`
	Model        string    `json:"model,omitempty"`
	AuthMode     string    `json:"auth_mode,omitempty"`
}

func Path() (string, error) {
	if path := strings.TrimSpace(os.Getenv("EASYBOT_AUTH_FILE")); path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}

	return filepath.Join(home, ".easybot", "auth.json"), nil
}

func Load() (File, error) {
	path, err := Path()
	if err != nil {
		return File{}, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}

	var data File
	if err := json.Unmarshal(raw, &data); err != nil {
		return File{}, fmt.Errorf("parse auth file %s: %w", path, err)
	}

	return data, nil
}

func Save(data File) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create auth directory: %w", err)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal auth file: %w", err)
	}
	raw = append(raw, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		return "", fmt.Errorf("write auth file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("move auth file into place: %w", err)
	}

	return path, nil
}
