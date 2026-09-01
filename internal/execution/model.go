package execution

type Execution struct {
	ID         string
	TaskID     string
	WorkerID   string
	Attempt    int64
	Status     string // running | completed | failed | timeout
	StartedAt  int64
	FinishedAt *int64
	Result     string
	Error      string
	CreatedAt  int64
}
