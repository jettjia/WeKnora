-- Semantic model sharing (数据建模共享): share published models to
-- organizations (共享空间), mirroring KB/agent sharing. Recipient-space
-- users query the model through a synthetic org-shared group injected in
-- their security context and an accessPolicy rule written at share time.

CREATE TABLE IF NOT EXISTS semantic_model_shares (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    model_id VARCHAR(36) NOT NULL,
    organization_id VARCHAR(36) NOT NULL,
    shared_by_user_id VARCHAR(64) NOT NULL DEFAULT '',
    source_tenant_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uidx_semantic_model_shares_model_org
    ON semantic_model_shares (model_id, organization_id);
CREATE INDEX IF NOT EXISTS idx_semantic_model_shares_org
    ON semantic_model_shares (organization_id);
CREATE INDEX IF NOT EXISTS idx_semantic_model_shares_model
    ON semantic_model_shares (model_id);
