-- Semantic actions (SQLite mirror of 000093 versioned).
CREATE TABLE IF NOT EXISTS semantic_actions (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    object_types TEXT NOT NULL DEFAULT '[]',
    input_schema TEXT NOT NULL DEFAULT '[]',
    preconditions TEXT NOT NULL DEFAULT '[]',
    backend TEXT NOT NULL DEFAULT '{}',
    allowed_groups TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS uidx_semantic_actions_tenant_name
    ON semantic_actions (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_semantic_actions_tenant
    ON semantic_actions (tenant_id);
