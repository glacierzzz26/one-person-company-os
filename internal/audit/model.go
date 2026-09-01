package audit

type Audit struct {
	ID         string
	EntityType string
	EntityID   string
	Action     string
	Actor      string
	Detail     string
	CreatedAt  int64
}
