package automation

// Repository provides data access for the automation module. All queries
// are tenant-scoped.

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound marks a missing row.
var ErrNotFound = errors.New("resource not found")

func translateNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// Repository provides data access for the automation module.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates the module repository.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// ListAutomations lists the tenant's non-deleted automations.
func (r *Repository) ListAutomations(ctx context.Context, tenantID uint64) ([]*Automation, error) {
	var out []*Automation
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at ASC").Find(&out).Error
	return out, err
}

// ListEnabledAutomations returns every enabled automation across tenants
// (the scheduler registers all of them; runs are tenant-scoped at execute time).
func (r *Repository) ListEnabledAutomations(ctx context.Context) ([]*Automation, error) {
	var out []*Automation
	err := r.db.WithContext(ctx).Where("enabled = ?", true).
		Order("created_at ASC").Find(&out).Error
	return out, err
}

// FindAutomation retrieves one automation by ID.
func (r *Repository) FindAutomation(ctx context.Context, tenantID uint64, id string) (*Automation, error) {
	var a Automation
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&a).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &a, nil
}

// FindAutomationByName retrieves one automation by slug.
func (r *Repository) FindAutomationByName(ctx context.Context, tenantID uint64, name string) (*Automation, error) {
	var a Automation
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND name = ?", tenantID, name).First(&a).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &a, nil
}

// FindAutomationAnyTenant retrieves one automation without tenant scoping —
// the Asynq worker needs this because the payload carries the tenant explicitly.
func (r *Repository) FindAutomationAnyTenant(ctx context.Context, id string) (*Automation, error) {
	var a Automation
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&a).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &a, nil
}

// SaveAutomation upserts an automation.
func (r *Repository) SaveAutomation(ctx context.Context, a *Automation) error {
	return r.db.WithContext(ctx).Save(a).Error
}

// DeleteAutomation soft-deletes an automation.
func (r *Repository) DeleteAutomation(ctx context.Context, a *Automation) error {
	return r.db.WithContext(ctx).Delete(a).Error
}

// CreateRun appends a run record.
func (r *Repository) CreateRun(ctx context.Context, run *AutomationRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

// UpdateRun persists run status changes.
func (r *Repository) UpdateRun(ctx context.Context, run *AutomationRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

// FindRun retrieves one run by ID.
func (r *Repository) FindRun(ctx context.Context, tenantID uint64, id string) (*AutomationRun, error) {
	var run AutomationRun
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&run).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &run, nil
}

// ListRuns lists runs of one automation, newest first.
func (r *Repository) ListRuns(ctx context.Context, tenantID uint64, automationID string, limit int) ([]*AutomationRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []*AutomationRun
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND automation_id = ?", tenantID, automationID).
		Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

// MarkOrphanRunsFailed reaps runs left in running/pending by a crashed or
// restarted process (called at scheduler startup). Returns affected rows.
func (r *Repository) MarkOrphanRunsFailed(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE automation_runs SET status = 'failed',
		    error = '进程中断, 孤儿运行被回收',
		    finished_at = NOW()
		 WHERE status IN ('running', 'pending')`)
	return res.RowsAffected, res.Error
}

// LatestRun returns the most recent run of one automation (any status).
func (r *Repository) LatestRun(ctx context.Context, tenantID uint64, automationID string) (*AutomationRun, error) {
	var run AutomationRun
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND automation_id = ?", tenantID, automationID).
		Order("created_at DESC").First(&run).Error
	if err != nil {
		return nil, translateNotFound(err)
	}
	return &run, nil
}
