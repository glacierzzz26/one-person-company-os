package repository

import (
	"context"
	"database/sql"

	"github.com/glacierzzz26/one-person-company-os/internal/memory"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/query"
)

func (s *Store) CreateMemory(ctx context.Context, m memory.Memory) (memory.Memory, error) {
	row, err := s.q.CreateMemory(ctx, query.CreateMemoryParams{
		ID: m.ID, CompanyID: m.CompanyID, Type: m.Type, Title: m.Title,
		Content: m.Content, Source: m.Source, Tags: m.Tags,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	})
	if err != nil {
		return memory.Memory{}, err
	}
	return toMemory(row), nil
}

func (s *Store) GetMemory(ctx context.Context, id string) (memory.Memory, error) {
	row, err := s.q.GetMemory(ctx, id)
	if err != nil {
		return memory.Memory{}, err
	}
	return toMemory(row), nil
}

func (s *Store) ListMemories(ctx context.Context, companyID, mtype string) ([]memory.Memory, error) {
	rows, err := s.q.ListMemories(ctx, query.ListMemoriesParams{
		CompanyFilter: companyID,
		CompanyID:     companyID,
		TypeFilter:    mtype,
		Type:          mtype,
	})
	if err != nil {
		return nil, err
	}
	out := make([]memory.Memory, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMemory(r))
	}
	return out, nil
}

// SearchMemories 全文搜索,join 回 memory 取完整行,按创建时间倒序。
// ≥3 字符走 FTS5 trigram;双字中文等短查询(trigram 不支持)走 LIKE 兜底。
func (s *Store) SearchMemories(ctx context.Context, companyID, q string) ([]memory.Memory, error) {
	if len([]rune(q)) < 3 {
		return s.searchMemoriesLike(ctx, companyID, q)
	}
	// FTS5 MATCH 走原 SQL(参数化)。
	sqlq := `SELECT m.id, m.company_id, m.type, m.title, m.content, m.source, m.tags, m.created_at, m.updated_at
FROM memory_fts JOIN memory m ON m.id = memory_fts.memory_id
WHERE memory_fts MATCH ?
ORDER BY m.created_at DESC`
	rows, err := s.db.QueryContext(ctx, sqlq, q)
	if err != nil {
		return nil, err
	}
	return scanMemories(rows)
}

// searchMemoriesLike trigram 兜底:标题/内容子串匹配(双字中文查询)。
func (s *Store) searchMemoriesLike(ctx context.Context, companyID, q string) ([]memory.Memory, error) {
	sqlq := `SELECT id, company_id, type, title, content, source, tags, created_at, updated_at
FROM memory
WHERE company_id = ? AND (title LIKE '%' || ? || '%' OR content LIKE '%' || ? || '%')
ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, sqlq, companyID, q, q)
	if err != nil {
		return nil, err
	}
	return scanMemories(rows)
}

func scanMemories(rows *sql.Rows) ([]memory.Memory, error) {
	defer rows.Close()
	out := []memory.Memory{}
	for rows.Next() {
		var m memory.Memory
		if err := rows.Scan(&m.ID, &m.CompanyID, &m.Type, &m.Title, &m.Content,
			&m.Source, &m.Tags, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func toMemory(r query.Memory) memory.Memory {
	return memory.Memory{
		ID: r.ID, CompanyID: r.CompanyID, Type: r.Type, Title: r.Title,
		Content: r.Content, Source: r.Source, Tags: r.Tags,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
