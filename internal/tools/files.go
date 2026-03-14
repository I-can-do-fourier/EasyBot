package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"easybot/internal/llm"
	"easybot/internal/security"
)

type listDirTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewListDirTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return listDirTool{policy: policy, audit: audit}
}

func (t listDirTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "list_dir",
			Description: "List files and directories inside a directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 500},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t listDirTool) SafetySummary(raw string) (bool, string) {
	return true, "read-only directory listing"
}

func (t listDirTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Path  string `json:"path"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	if args.Limit <= 0 || args.Limit > 500 {
		args.Limit = 200
	}
	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if len(entries) > args.Limit {
		entries = entries[:args.Limit]
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		out = append(out, map[string]any{
			"name":  entry.Name(),
			"isDir": entry.IsDir(),
			"size":  size,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	t.audit.Write("list_dir", "ok", path)
	return Result{OK: true, Data: out}
}

type readFileTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewReadFileTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return readFileTool{policy: policy, audit: audit}
}

func (t readFileTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "read_file",
			Description: "Read a UTF-8 text file up to a bounded number of bytes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string"},
					"max_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": 200000},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t readFileTool) SafetySummary(raw string) (bool, string) { return true, "read-only file access" }

func (t readFileTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Path     string `json:"path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	if args.MaxBytes <= 0 || args.MaxBytes > 200000 {
		args.MaxBytes = 32000
	}
	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	binary := looksBinary(data)
	if len(data) > args.MaxBytes {
		data = data[:args.MaxBytes]
	}
	t.audit.Write("read_file", "ok", path)
	return Result{OK: true, Data: map[string]any{"path": path, "binary": binary, "content": string(data)}}
}

type findFilesTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewFindFilesTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return findFilesTool{policy: policy, audit: audit}
}

func (t findFilesTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "find_files",
			Description: "Find files by filename substring under a root directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":  map[string]any{"type": "string"},
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200},
				},
				"required": []string{"root", "query"},
			},
		},
	}
}

func (t findFilesTool) SafetySummary(raw string) (bool, string) { return true, "read-only file search" }

func (t findFilesTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Root  string `json:"root"`
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	if args.Limit <= 0 || args.Limit > 200 {
		args.Limit = 50
	}
	root, err := t.policy.Resolve(args.Root)
	if err != nil {
		return Result{Error: err.Error()}
	}
	query := strings.ToLower(args.Query)
	matches := make([]string, 0, args.Limit)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(matches) >= args.Limit {
			return filepath.SkipAll
		}
		if strings.Contains(strings.ToLower(d.Name()), query) {
			matches = append(matches, path)
		}
		return nil
	})
	sort.Strings(matches)
	t.audit.Write("find_files", "ok", fmt.Sprintf("%s query=%q", root, args.Query))
	return Result{OK: true, Data: matches}
}

type searchFileContentTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewSearchFileContentTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return searchFileContentTool{policy: policy, audit: audit}
}

func (t searchFileContentTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "search_file_content",
			Description: "Search text file contents for a substring under a root directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":           map[string]any{"type": "string"},
					"query":          map[string]any{"type": "string"},
					"limit":          map[string]any{"type": "integer", "minimum": 1, "maximum": 200},
					"max_file_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": 200000},
				},
				"required": []string{"root", "query"},
			},
		},
	}
}

func (t searchFileContentTool) SafetySummary(raw string) (bool, string) {
	return true, "read-only content search"
}

func (t searchFileContentTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Root         string `json:"root"`
		Query        string `json:"query"`
		Limit        int    `json:"limit"`
		MaxFileBytes int    `json:"max_file_bytes"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	if args.Limit <= 0 || args.Limit > 200 {
		args.Limit = 50
	}
	if args.MaxFileBytes <= 0 || args.MaxFileBytes > 200000 {
		args.MaxFileBytes = 65536
	}
	root, err := t.policy.Resolve(args.Root)
	if err != nil {
		return Result{Error: err.Error()}
	}
	query := strings.ToLower(args.Query)
	results := make([]map[string]any, 0, args.Limit)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if len(results) >= args.Limit {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil || info.Size() > int64(args.MaxFileBytes) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || looksBinary(data) {
			return nil
		}
		content := string(data)
		lower := strings.ToLower(content)
		idx := strings.Index(lower, query)
		if idx < 0 {
			return nil
		}
		start := max(idx-80, 0)
		end := min(idx+len(args.Query)+80, len(content))
		results = append(results, map[string]any{
			"path":    path,
			"snippet": strings.TrimSpace(content[start:end]),
		})
		return nil
	})
	t.audit.Write("search_file_content", "ok", fmt.Sprintf("%s query=%q", root, args.Query))
	return Result{OK: true, Data: results}
}

type statFileTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewStatFileTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return statFileTool{policy: policy, audit: audit}
}

func (t statFileTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "stat_file",
			Description: "Return metadata for a file or directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t statFileTool) SafetySummary(raw string) (bool, string) {
	return true, "read-only metadata access"
}

func (t statFileTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	info, err := os.Stat(path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	t.audit.Write("stat_file", "ok", path)
	return Result{OK: true, Data: map[string]any{
		"path":     path,
		"name":     info.Name(),
		"size":     info.Size(),
		"mode":     info.Mode().String(),
		"is_dir":   info.IsDir(),
		"modified": info.ModTime(),
	}}
}

type writeFileTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewWriteFileTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return writeFileTool{policy: policy, audit: audit}
}

func (t writeFileTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "write_file",
			Description: "Write text content to a file inside allowed roots. This should be used sparingly and only when explicitly needed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

func (t writeFileTool) SafetySummary(raw string) (bool, string) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return false, "file write requires approval"
	}
	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return false, "file write requires approval"
	}
	if _, err := os.Stat(path); err == nil {
		return false, "changing an existing file requires explicit human approval"
	}
	return false, "creating a new file requires explicit human approval"
}

func (t writeFileTool) Run(_ context.Context, raw string) Result {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return Result{Error: err.Error()}
	}
	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	_, statErr := os.Stat(path)
	existed := statErr == nil
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return Result{Error: err.Error()}
	}
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return Result{Error: err.Error()}
	}
	t.audit.Write("write_file", "ok", path)
	return Result{OK: true, Data: map[string]any{"path": path, "bytes": len(args.Content), "created": !existed}}
}

func looksBinary(data []byte) bool {
	maxScan := min(len(data), 1024)
	for i := 0; i < maxScan; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
