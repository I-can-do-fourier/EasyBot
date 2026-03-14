package agent

type ApprovalOptions struct {
	AutoApproveSafe bool `json:"auto_approve_safe"`
}

type HistoryTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Message  string          `json:"message"`
	Approval ApprovalOptions `json:"approval"`
	History  []HistoryTurn   `json:"history,omitempty"`
}

type Response struct {
	Output   string        `json:"output"`
	Steps    int           `json:"steps"`
	Mode     Mode          `json:"mode"`
	ToolRuns []ToolRunInfo `json:"tool_runs,omitempty"`
}

type ToolRunInfo struct {
	Name     string `json:"name"`
	Approved bool   `json:"approved"`
	Status   string `json:"status"`
}
