-- Automation module (自动化): scheduled agent runs. All tables additive;
-- upstream merges never touch them. See internal/automation/README.md.

CREATE TABLE IF NOT EXISTS automations (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    agent_id VARCHAR(36) NOT NULL,
    query_template TEXT NOT NULL DEFAULT '',
    schedule_cron VARCHAR(64) NOT NULL,
    schedule_tz VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    overlap_policy VARCHAR(16) NOT NULL DEFAULT 'skip',
    timeout_minutes INT NOT NULL DEFAULT 15,
    notify_config JSONB,
    created_by VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uidx_automations_tenant_name
    ON automations (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_automations_tenant
    ON automations (tenant_id);

CREATE TABLE IF NOT EXISTS automation_runs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    automation_id VARCHAR(36) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    trigger_type VARCHAR(16) NOT NULL DEFAULT 'cron',
    session_id VARCHAR(36),
    output_summary TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_automation_runs_automation
    ON automation_runs (automation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_automation_runs_tenant
    ON automation_runs (tenant_id);
