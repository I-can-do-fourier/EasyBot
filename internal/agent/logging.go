package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func newAppLogger(cfg Config) (*log.Logger, io.Closer, error) {
	if isDisabledLogLevel(cfg.LogLevel) {
		return log.New(io.Discard, "", log.LstdFlags), nil, nil
	}

	writer, closer, err := newFileWriter(cfg.AppLogPath)
	if err != nil {
		return nil, nil, err
	}
	return log.New(writer, "", log.LstdFlags), closer, nil
}

func newFileWriter(path string) (io.Writer, io.Closer, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return io.Discard, nil, nil
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, nil, err
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

func isDisabledLogLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "off", "none", "silent", "disabled":
		return true
	default:
		return false
	}
}

func runnerLogSummary(cfg Config) string {
	return fmt.Sprintf(
		"mode=%s auth_mode=%s base_url=%q model=%q allowed_roots=%d max_steps=%d step_timeout=%s app_log=%q audit_log=%q tool_output_max_bytes=%d api_key=%s access_token=%s account_id=%s",
		cfg.Mode,
		cfg.AuthMode,
		cfg.BaseURL,
		cfg.Model,
		len(cfg.AllowedRoots),
		cfg.MaxSteps,
		cfg.StepTimeout,
		cfg.AppLogPath,
		cfg.AuditLogPath,
		cfg.ToolOutputMaxBytes,
		cfg.APIKey,
		cfg.AccessToken,
		cfg.AccountID,
	)
}
