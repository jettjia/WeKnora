// Package automation implements the automation module (自动化): scheduled
// agent runs. An automation binds an agent to a cron schedule and a query
// template; the scheduler enqueues runs, a worker executes them through the
// standard AgentQA pipeline on a fresh session, and each run is auditable
// with full conversation replay. See README.md in this package.
package automation

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// Automation lifecycle / run status values.
const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusSuccess   = "success"
	RunStatusFailed    = "failed"
	RunStatusTimeout   = "timeout"
	RunStatusSkipped   = "skipped"
	RunStatusCancelled = "cancelled"

	TriggerCron   = "cron"
	TriggerManual = "manual"

	OverlapSkip  = "skip"
	OverlapQueue = "queue"

	DefaultTimeoutMinutes = 15
	DefaultTimezone       = "Asia/Shanghai"
)

// Automation is one scheduled agent task.
type Automation struct {
	ID        string `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID  uint64 `json:"tenant_id" gorm:"index"`
	// Name is the unique slug within the tenant.
	Name string `json:"name" gorm:"type:varchar(64)"`
	// Title is the display name; defaults to Name when empty.
	Title       string `json:"title" gorm:"type:varchar(255)"`
	Description string `json:"description" gorm:"type:text"`
	// AgentID is the custom agent executed on every run.
	AgentID string `json:"agent_id" gorm:"type:varchar(36)"`
	// QueryTemplate is the instruction sent on each run. Supports
	// {{date}} {{time}} {{datetime}} {{weekday}} variables.
	QueryTemplate string `json:"query_template" gorm:"type:text"`
	// ScheduleCron is a 5-field cron expression (robfig/cron v3 syntax,
	// CRON_TZ prefix allowed but stored separately via ScheduleTZ).
	ScheduleCron string `json:"schedule_cron" gorm:"type:varchar(64)"`
	// ScheduleTZ is the IANA timezone the cron fires in.
	ScheduleTZ    string `json:"schedule_tz" gorm:"type:varchar(64);default:'Asia/Shanghai'"`
	Enabled       bool   `json:"enabled" gorm:"default:true"`
	// OverlapPolicy: skip (default) or queue when the previous run holds the lock.
	OverlapPolicy  string `json:"overlap_policy" gorm:"type:varchar(16);default:'skip'"`
	TimeoutMinutes int    `json:"timeout_minutes" gorm:"default:15"`
	// NotifyConfig is reserved for v2 delivery targets (webhook / IM).
	NotifyConfig types.JSON `json:"notify_config,omitempty" gorm:"type:jsonb"`
	CreatedBy    string     `json:"created_by" gorm:"type:varchar(64)"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// NextRuns are computed schedule previews (never persisted).
	NextRuns []time.Time `json:"next_runs,omitempty" gorm:"-"`
	// LastRun is the most recent run summary for list views (never persisted).
	LastRun *RunSummary `json:"last_run,omitempty" gorm:"-"`
}

// TableName specifies the table name for Automation.
func (Automation) TableName() string { return "automations" }

// BeforeCreate hook to generate UUID.
func (a *Automation) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	return nil
}

// RunSummary is the abbreviated last-run info shown on list views.
type RunSummary struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	Trigger   string     `json:"trigger"`
	StartedAt *time.Time `json:"started_at"`
	Duration  int64      `json:"duration_ms"`
}

// AutomationRun is one execution of an automation.
type AutomationRun struct {
	ID           string  `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID     uint64  `json:"tenant_id" gorm:"index"`
	AutomationID string  `json:"automation_id" gorm:"type:varchar(36);index:idx_automation_runs_automation"`
	Status       string  `json:"status" gorm:"type:varchar(16);default:'pending'"`
	TriggerType  string  `json:"trigger_type" gorm:"type:varchar(16);default:'cron'"`
	SessionID    string  `json:"session_id" gorm:"type:varchar(36)"`
	OutputSummary string `json:"output_summary" gorm:"type:text"`
	Error        string  `json:"error" gorm:"type:text"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	DurationMs   int64      `json:"duration_ms"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TableName specifies the table name for AutomationRun.
func (AutomationRun) TableName() string { return "automation_runs" }

// BeforeCreate hook to generate UUID.
func (r *AutomationRun) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// RenderTemplate expands the supported variables in a query template.
func RenderTemplate(tpl string, now time.Time) string {
	return strings.NewReplacer(
		"{{date}}", now.Format("2006-01-02"),
		"{{time}}", now.Format("15:04"),
		"{{datetime}}", now.Format("2006-01-02 15:04"),
		"{{weekday}}", now.Format("Monday"),
	).Replace(tpl)
}
