package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PathPolicy struct {
	roots []string
}

func NewPathPolicy(roots []string) PathPolicy {
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		if abs, err := filepath.Abs(root); err == nil {
			cleaned = append(cleaned, filepath.Clean(abs))
		}
	}
	return PathPolicy{roots: cleaned}
}

func (p PathPolicy) Resolve(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	for _, root := range p.roots {
		if abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed roots", path)
}
