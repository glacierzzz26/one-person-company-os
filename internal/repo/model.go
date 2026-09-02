package repo

// Repo 研发仓库登记(Phase 6.3)。repo_url 供 GitHub issue 归属解析(owner/repo),
// workspace_path 是 git/file 工具的工作区边界(沿既有 workspace 强制语义)。
type Repo struct {
	ID            string `json:"id"`
	CompanyID     string `json:"company_id"`
	Name          string `json:"name"`
	RepoURL       string `json:"repo_url"`
	WorkspacePath string `json:"workspace_path"`
	CreatedAt     int64  `json:"created_at"`
}

// IssueSync 是通道 B(研发 Intake)的同步账本:一条 issue 一次处置。
// disposition: direct_work | ask | skip | merge;direct_work 会 create 一条
// engineering task(挂 task_id)。UNIQUE(repo_id, issue_number) 原生去重,
// webhook 优先 + 轮询兜底不会重复处理同一 issue。
type IssueSync struct {
	ID          string  `json:"id"`
	CompanyID   string  `json:"company_id"`
	RepoID      string  `json:"repo_id"`
	IssueNumber int64   `json:"issue_number"`
	Title       string  `json:"title"`
	Disposition string  `json:"disposition"`
	TaskID      *string `json:"task_id"` // direct_work 建出的 engineering task;其余处置为空
	Note        string  `json:"note"`    // skip 理由 / ask 追问 / merge 批注
	CreatedAt   int64   `json:"created_at"`
}
