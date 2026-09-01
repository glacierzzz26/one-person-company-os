-- Phase 5.2: decision 表 — 人类/系统决策留痕(治理链闭环 Policy→Permission→Approval→Audit→Decision)

CREATE TABLE decision (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    title       TEXT NOT NULL,
    kind        TEXT NOT NULL,   -- approval | goal | strategy | policy_change | capital | manual
    status      TEXT NOT NULL,   -- made | pending | executed
    body        TEXT NOT NULL DEFAULT '',
    decided_by  TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT '',   -- approval:<id> / manual
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_decision_company ON decision (company_id);
