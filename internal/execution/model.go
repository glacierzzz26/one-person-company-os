package execution

type Execution struct {
	ID         string `json:"id"`
	TaskID     string `json:"task_id"`
	WorkerID   string `json:"worker_id"`
	Attempt    int64  `json:"attempt"`
	Status     string `json:"status"` // running | completed | failed | timeout
	StartedAt  int64  `json:"started_at"`
	FinishedAt *int64 `json:"finished_at"`
	Result     string `json:"result"`
	Error      string `json:"error"`
	CreatedAt  int64  `json:"created_at"`
}
