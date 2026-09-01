package repository

import (
	"database/sql"

	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

type Store struct {
	db *sql.DB
	q  *query.Queries
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, q: query.New(db)}
}
