CREATE TABLE company (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    vision     TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE capability (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    code        TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE (company_id, code)
);

CREATE TABLE agent (
    id           TEXT PRIMARY KEY,
    capability_id TEXT NOT NULL REFERENCES capability(id),
    name         TEXT NOT NULL,
    role         TEXT NOT NULL,
    model_hint   TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE policy (
    id         TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES company(id),
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    statement  TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE permission (
    id         TEXT PRIMARY KEY,
    policy_id  TEXT NOT NULL REFERENCES policy(id),
    subject    TEXT NOT NULL,
    action     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE workflow (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES company(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    definition  TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE task (
    id            TEXT PRIMARY KEY,
    company_id    TEXT NOT NULL REFERENCES company(id),
    capability_id TEXT REFERENCES capability(id),
    workflow_id   TEXT REFERENCES workflow(id),
    agent_id      TEXT REFERENCES agent(id),
    title         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    priority      INTEGER NOT NULL DEFAULT 0,
    attempt       INTEGER NOT NULL DEFAULT 0,
    risk          TEXT NOT NULL DEFAULT 'low',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

CREATE TABLE audit (
    id          TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    action      TEXT NOT NULL,
    actor       TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE INDEX idx_audit_entity ON audit (entity_type, entity_id);
