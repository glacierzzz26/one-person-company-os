-- Phase 5.1 down: memory 表 + FTS5 索引

DROP TRIGGER IF EXISTS memory_au;
DROP TRIGGER IF EXISTS memory_ad;
DROP TRIGGER IF EXISTS memory_ai;
DROP TABLE IF EXISTS memory_fts;
DROP TABLE IF EXISTS memory;
