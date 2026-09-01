package repository

import (
	"database/sql"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

type Store struct {
	db *sql.DB
	q  *query.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: query.New(db)}
}

func now() int64 { return time.Now().Unix() }
