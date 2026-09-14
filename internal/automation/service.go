package automation

// Service: business operations for automations. Mutations notify the
// scheduler through the ScheduleUpdater hook so cron entries stay in sync
// without restarting. RunNow enqueues a manual run through the task queue.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ErrConflict marks a business conflict (e.g. slug already exists).
var ErrConflict = errors.New("conflict")

// ConflictError carries a user-facing conflict message.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string  { return e.Msg }
func (e *ConflictError) Unwrap() error  { return ErrConflict }

// ScheduleUpdater is implemented by the scheduler; the service calls it
// after every mutation so cron entries stay in sync. RunNow bypasses the
// schedule and enqueues directly through the task enqueuer.
type ScheduleUpdater interface {
	OnAutomationChanged(a *Automation)
	OnAutomationRemoved(id string)
}

// cronParser matches what scheduler.go registers: 5-field + descriptors,
// no seconds.
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// ValidateSchedule checks the cron expression and returns the next `n`
// fire times in the given timezone (for the UI preview).
func ValidateSchedule(cronExpr, tz string, n int) ([]time.Time, error) {
	if strings.TrimSpace(cronExpr) == "" {
		return nil, fmt.Errorf("schedule cron is required")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	sched, err := cronParser.Parse(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}
	now := time.Now().In(loc)
	out := make([]time.Time, 0, n)
	t := now
	for i := 0; i < n; i++ {
		t = sched.Next(t)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out, nil
}

// Service implements the automation business logic.
type Service struct {
	repo     *Repository
	enqueuer interfaces.TaskEnqueuer
	updater  ScheduleUpdater // may be nil until the scheduler starts
}

// NewService builds the automation service.
func NewService(repo *Repository, enqueuer interfaces.TaskEnqueuer) *Service {
	return &Service{repo: repo, enqueuer: enqueuer}
}

// SetScheduleUpdater wires the scheduler hook (called once at startup).
func (s *Service) SetScheduleUpdater(u ScheduleUpdater) { s.updater = u }

func (s *Service) notifyChanged(a *Automation) {
	if s.updater != nil {
		s.updater.OnAutomationChanged(a)
	}
}

func (s *Service) notifyRemoved(id string) {
	if s.updater != nil {
		s.updater.OnAutomationRemoved(id)
	}
}

// AutomationInput is the create/update payload.
type AutomationInput struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	AgentID        string `json:"agent_id"`
	QueryTemplate  string `json:"query_template"`
	ScheduleCron   string `json:"schedule_cron"`
	ScheduleTZ     string `json:"schedule_tz"`
	Enabled        *bool  `json:"enabled,omitempty"`
	OverlapPolicy  string `json:"overlap_policy,omitempty"`
	TimeoutMinutes int    `json:"timeout_minutes,omitempty"`
}

func validateInput(in *AutomationInput) error {
	if !isValidSlug(in.Name) {
		return fmt.Errorf("name must match ^[a-z][a-z0-9_]*$")
	}
	if in.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if strings.TrimSpace(in.QueryTemplate) == "" {
		return fmt.Errorf("query_template is required")
	}
	if _, err := ValidateSchedule(in.ScheduleCron, in.ScheduleTZ, 1); err != nil {
		return err
	}
	if in.OverlapPolicy != "" && in.OverlapPolicy != OverlapSkip && in.OverlapPolicy != OverlapQueue {
		return fmt.Errorf("overlap_policy must be skip or queue")
	}
	if in.TimeoutMinutes < 0 || in.TimeoutMinutes > 720 {
		return fmt.Errorf("timeout_minutes must be between 0 and 720")
	}
	return nil
}

func isValidSlug(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i, c := range name {
		if c == '_' {
			continue
		}
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			if i == 0 && c >= '0' && c <= '9' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

// CreateAutomation validates and persists a new automation, registering its
// schedule when enabled.
func (s *Service) CreateAutomation(ctx context.Context, userID string, tenantID uint64, in *AutomationInput) (*Automation, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindAutomationByName(ctx, tenantID, in.Name); err == nil {
		return nil, &ConflictError{Msg: fmt.Sprintf("automation name %s already exists", in.Name)}
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	a := &Automation{
		TenantID: tenantID, Name: in.Name,
		Title: in.Title, Description: in.Description,
		AgentID: in.AgentID, QueryTemplate: in.QueryTemplate,
		ScheduleCron: in.ScheduleCron,
		ScheduleTZ:   in.ScheduleTZ,
		Enabled:      in.Enabled == nil || *in.Enabled,
		OverlapPolicy: in.OverlapPolicy,
		TimeoutMinutes: in.TimeoutMinutes,
		CreatedBy:    userID,
	}
	if a.Title == "" {
		a.Title = a.Name
	}
	if a.ScheduleTZ == "" {
		a.ScheduleTZ = DefaultTimezone
	}
	if a.OverlapPolicy == "" {
		a.OverlapPolicy = OverlapSkip
	}
	if a.TimeoutMinutes == 0 {
		a.TimeoutMinutes = DefaultTimeoutMinutes
	}
	if err := s.repo.SaveAutomation(ctx, a); err != nil {
		return nil, err
	}
	if a.Enabled {
		s.notifyChanged(a)
	}
	return a, nil
}

// UpdateAutomation applies non-empty fields; empty schedule/timezone keep
// stored values so partial updates stay simple.
func (s *Service) UpdateAutomation(ctx context.Context, tenantID uint64, id string, in *AutomationInput) (*Automation, error) {
	a, err := s.repo.FindAutomation(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" && in.Name != a.Name {
		if _, err := s.repo.FindAutomationByName(ctx, tenantID, in.Name); err == nil {
			return nil, &ConflictError{Msg: fmt.Sprintf("automation name %s already exists", in.Name)}
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		a.Name = in.Name
	}
	if in.Title != "" {
		a.Title = in.Title
	}
	if in.Description != "" {
		a.Description = in.Description
	}
	if in.AgentID != "" {
		a.AgentID = in.AgentID
	}
	if in.QueryTemplate != "" {
		a.QueryTemplate = in.QueryTemplate
	}
	if in.ScheduleCron != "" {
		a.ScheduleCron = in.ScheduleCron
	}
	if in.ScheduleTZ != "" {
		a.ScheduleTZ = in.ScheduleTZ
	}
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	}
	if in.OverlapPolicy != "" {
		a.OverlapPolicy = in.OverlapPolicy
	}
	if in.TimeoutMinutes > 0 {
		a.TimeoutMinutes = in.TimeoutMinutes
	}
	if err := validateInput(&AutomationInput{
		Name: a.Name, AgentID: a.AgentID, QueryTemplate: a.QueryTemplate,
		ScheduleCron: a.ScheduleCron, ScheduleTZ: a.ScheduleTZ,
		OverlapPolicy: a.OverlapPolicy, TimeoutMinutes: a.TimeoutMinutes,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.SaveAutomation(ctx, a); err != nil {
		return nil, err
	}
	s.notifyChanged(a)
	return a, nil
}

// DeleteAutomation soft-deletes and unregisters the schedule.
func (s *Service) DeleteAutomation(ctx context.Context, tenantID uint64, id string) error {
	a, err := s.repo.FindAutomation(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteAutomation(ctx, a); err != nil {
		return err
	}
	s.notifyRemoved(id)
	return nil
}

// GetAutomation returns one automation enriched with schedule preview and
// the latest run summary.
func (s *Service) GetAutomation(ctx context.Context, tenantID uint64, id string) (*Automation, error) {
	a, err := s.repo.FindAutomation(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	s.enrich(ctx, tenantID, a)
	return a, nil
}

// ListAutomations returns the tenant's automations enriched for list views.
func (s *Service) ListAutomations(ctx context.Context, tenantID uint64) ([]*Automation, error) {
	list, err := s.repo.ListAutomations(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		s.enrich(ctx, tenantID, a)
	}
	return list, nil
}

func (s *Service) enrich(ctx context.Context, tenantID uint64, a *Automation) {
	if runs, err := ValidateSchedule(a.ScheduleCron, a.ScheduleTZ, 3); err == nil {
		a.NextRuns = runs
	}
	if run, err := s.repo.LatestRun(ctx, tenantID, a.ID); err == nil {
		a.LastRun = &RunSummary{
			ID: run.ID, Status: run.Status, Trigger: run.TriggerType,
			StartedAt: run.StartedAt, Duration: run.DurationMs,
		}
	}
}

// ListRuns returns the run history of one automation.
func (s *Service) ListRuns(ctx context.Context, tenantID uint64, automationID string, limit int) ([]*AutomationRun, error) {
	if _, err := s.repo.FindAutomation(ctx, tenantID, automationID); err != nil {
		return nil, err
	}
	return s.repo.ListRuns(ctx, tenantID, automationID, limit)
}

// RunNow enqueues a manual run through the task queue.
func (s *Service) RunNow(ctx context.Context, tenantID uint64, id string) error {
	a, err := s.repo.FindAutomation(ctx, tenantID, id)
	if err != nil {
		return err
	}
	return enqueueRun(ctx, s.enqueuer, a, TriggerManual)
}

// logSkip is a helper for the runner to record skip decisions consistently.
func logSkip(ctx context.Context, automationID, reason string) {
	logger.Infof(ctx, "[automation] run skipped, automation=%s reason=%s", automationID, reason)
}
