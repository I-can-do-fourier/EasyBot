package security

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type AuditLog struct {
	w  io.Writer
	mu sync.Mutex
}

type AuditEvent struct {
	Time   time.Time `json:"time"`
	Tool   string    `json:"tool"`
	Status string    `json:"status"`
	Detail string    `json:"detail"`
}

func NewAuditLog(w io.Writer) *AuditLog {
	return &AuditLog{w: w}
}

func (a *AuditLog) Write(tool, status, detail string) {
	if a == nil || a.w == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = json.NewEncoder(a.w).Encode(AuditEvent{
		Time:   time.Now().UTC(),
		Tool:   tool,
		Status: status,
		Detail: detail,
	})
}
