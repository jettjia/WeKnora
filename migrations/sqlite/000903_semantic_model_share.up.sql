-- Semantic model sharing (数据建模共享). SQLite mirror of 000903 versioned.
CREATE TABLE IF NOT EXISTS semantic_model_shares (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    model_id TEXT NOT NULL,
    organization_id TEXT NOT NULL,
    shared_by_user_id TEXT NOT NULL DEFAULT '',
    source_tenant_id INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS uidx_semantic_model_shares_model_org
    ON semantic_model_shares (model_id, organization_id);
CREATE INDEX IF NOT EXISTS idx_semantic_model_shares_org
    ON semantic_model_shares (organization_id);
CREATE INDEX IF NOT EXISTS idx_semantic_model_shares_model
    ON semantic_model_shares (model_id);
