-- Automation module (自动化): scheduled agent runs. SQLite mirror of
-- 000902 versioned. See internal/automation/README.md.

CREATE TABLE IF NOT EXISTS automations (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL,
    query_template TEXT NOT NULL DEFAULT '',
    schedule_cron TEXT NOT NULL,
    schedule_tz TEXT NOT NULL DEFAULT 'Asia/Shanghai',
    enabled INTEGER NOT NULL DEFAULT 1,
    overlap_policy TEXT NOT NULL DEFAULT 'skip',
    timeout_minutes INTEGER NOT NULL DEFAULT 15,
    notify_config TEXT,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS uidx_automations_tenant_name
    ON automations (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_automations_tenant
    ON automations (tenant_id);

CREATE TABLE IF NOT EXISTS automation_runs (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    automation_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    trigger_type TEXT NOT NULL DEFAULT 'cron',
    session_id TEXT,
    output_summary TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at TEXT,
    finished_at TEXT,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_automation_runs_automation
    ON automation_runs (automation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_automation_runs_tenant
    ON automation_runs (tenant_id);
