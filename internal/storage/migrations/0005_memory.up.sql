-- Phase 5.1: memory 表 + FTS5 全文索引(公司知识沉淀,基线 §4.10)

CREATE TABLE memory (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    type        TEXT NOT NULL,   -- lesson | knowledge | project_context | decision_ref | architecture | convention | task_history
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',  -- 血缘:workflow:<id> / task:<id> / approval:<id> / manual
    tags        TEXT NOT NULL DEFAULT '',  -- 空格分隔
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_memory_company ON memory (company_id);

-- FTS5 独立索引表(memory_id 列用于 join 回 memory 取完整行)。
-- 分词器用 trigram:支持中英文 ≥3 字符子串匹配;双字中文(反馈/定价)等短查询由 CLI 走 LIKE 兜底。
CREATE VIRTUAL TABLE memory_fts USING fts5(memory_id, title, content, tokenize='trigram');

-- 触发器保持 FTS 与 memory 同步(INSERT/DELETE/UPDATE)。
CREATE TRIGGER memory_ai AFTER INSERT ON memory BEGIN
    INSERT INTO memory_fts(memory_id, title, content) VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER memory_ad AFTER DELETE ON memory BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, memory_id, title, content)
    VALUES ('delete', old.rowid, old.id, old.title, old.content);
END;

CREATE TRIGGER memory_au AFTER UPDATE ON memory BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, memory_id, title, content)
    VALUES ('delete', (SELECT rowid FROM memory_fts WHERE memory_id = old.id), old.id, old.title, old.content);
    INSERT INTO memory_fts(memory_id, title, content) VALUES (new.id, new.title, new.content);
END;
