package permission

type Permission struct {
	ID        string
	PolicyID  string
	Subject   string
	Action    string
	Resource  string
	CreatedAt int64
}
