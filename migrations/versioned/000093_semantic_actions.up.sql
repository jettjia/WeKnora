-- Semantic actions (数据建模 Action): declarative operation types attached
-- to Cube models. An action declares input schema, preconditions, a webhook
-- backing, and data-group permissions. See internal/semantic/README.md.

CREATE TABLE IF NOT EXISTS semantic_actions (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    object_types JSONB NOT NULL DEFAULT '[]',
    input_schema JSONB NOT NULL DEFAULT '[]',
    preconditions JSONB NOT NULL DEFAULT '[]',
    backend JSONB NOT NULL DEFAULT '{}',
    allowed_groups JSONB NOT NULL DEFAULT '[]',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_by VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uidx_semantic_actions_tenant_name
    ON semantic_actions (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_semantic_actions_tenant
    ON semantic_actions (tenant_id);
