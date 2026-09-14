package automation

// Scheduler: in-process robfig/cron that fires enqueues only. Actual
// execution happens in an Asynq worker (runner.go) so slow agent runs never
// block scheduling and multi-replica deployments get single-flight behavior
// through the task queue.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AsynqTaskType and QueueName route run tasks through the platform task
// topology (types/task.go: dedicated automation queue on the maintenance
// worker pool).
const (
	AsynqTaskType = types.TypeAutomationRun
	QueueName     = types.QueueAutomation
)

// MaxRetry is always zero by design: automation runs often carry side
// effects, silent retries would duplicate them. Failures surface in the run
// history for manual re-runs.
const MaxRetry = 0

// Scheduler maintains one cron entry per enabled automation. Fires enqueue
// a task through the platform TaskEnqueuer; entries are re-registered on
// every automation mutation via the ScheduleUpdater hook.
type Scheduler struct {
	cron     *cron.Cron
	repo     *Repository
	enqueuer interfaces.TaskEnqueuer

	mu       sync.Mutex
	entryIDs map[string]cron.EntryID // automation id → cron entry
}

// NewScheduler builds the scheduler (not started; call Start).
func NewScheduler(repo *Repository, enqueuer interfaces.TaskEnqueuer) *Scheduler {
	return &Scheduler{
		cron:     cron.New(),
		repo:     repo,
		enqueuer: enqueuer,
		entryIDs: make(map[string]cron.EntryID),
	}
}

// Start loads every enabled automation and begins firing.
func (s *Scheduler) Start(ctx context.Context) error {
	list, err := s.repo.ListEnabledAutomations(ctx)
	if err != nil {
		return fmt.Errorf("automation scheduler load failed: %w", err)
	}
	s.cron.Start()
	for _, a := range list {
		s.upsertEntry(a)
	}
	logger.Infof(ctx, "[automation] scheduler started with %d enabled automations", len(list))
	return nil
}

// Stop halbs the cron loop.
func (s *Scheduler) Stop() { s.cron.Stop() }

// OnAutomationChanged re-registers (or removes) the cron entry for one
// automation. Implements service.ScheduleUpdater.
func (s *Scheduler) OnAutomationChanged(a *Automation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entryIDs[a.ID]; ok {
		s.cron.Remove(id)
		delete(s.entryIDs, a.ID)
	}
	if !a.Enabled {
		return
	}
	s.addEntryLocked(a)
}

// OnAutomationRemoved unregisters the cron entry. Implements
// service.ScheduleUpdater.
func (s *Scheduler) OnAutomationRemoved(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entryIDs[id]; ok {
		s.cron.Remove(entry)
		delete(s.entryIDs, id)
	}
}

func (s *Scheduler) upsertEntry(a *Automation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entryIDs[a.ID]; ok {
		s.cron.Remove(id)
		delete(s.entryIDs, a.ID)
	}
	if !a.Enabled {
		return
	}
	s.addEntryLocked(a)
}

func (s *Scheduler) addEntryLocked(a *Automation) {
	spec := fmt.Sprintf("CRON_TZ=%s %s", a.ScheduleTZ, a.ScheduleCron)
	automationID := a.ID
	entry, err := s.cron.AddFunc(spec, func() { s.fire(automationID) })
	if err != nil {
		// Validation happens at CRUD time; reaching here means a bad spec
		// slipped through (e.g. hand-edited row) — log and continue.
		logger.Errorf(context.Background(),
			"[automation] invalid cron for automation=%s spec=%q: %v", automationID, spec, err)
		return
	}
	s.entryIDs[automationID] = entry
}

// fire enqueues one run task. The scheduler never executes the agent
// directly — the Asynq worker owns execution.
func (s *Scheduler) fire(automationID string) {
	ctx := context.Background()
	a, err := s.repo.FindAutomationAnyTenant(ctx, automationID)
	if err != nil {
		logger.Warnf(ctx, "[automation] fire for missing automation=%s: %v", automationID, err)
		return
	}
	if !a.Enabled {
		return
	}
	if err := enqueueRun(ctx, s.enqueuer, a, TriggerCron); err != nil {
		logger.Errorf(ctx, "[automation] enqueue failed automation=%s: %v", automationID, err)
	}
}

// enqueueRun builds the task payload and pushes it through the enqueuer.
// Shared by cron fire and manual RunNow. No auto-retry by design; the task
// timeout mirrors the automation's own timeout plus a scheduling buffer.
func enqueueRun(ctx context.Context, enqueuer interfaces.TaskEnqueuer, a *Automation, trigger string) error {
	payload := fmt.Sprintf(`{"automation_id":%q,"tenant_id":%d,"trigger":%q}`,
		a.ID, a.TenantID, trigger)
	task := asynq.NewTask(AsynqTaskType, []byte(payload))
	opts := []asynq.Option{
		asynq.Queue(QueueName),
		asynq.MaxRetry(MaxRetry),
		asynq.Timeout(time.Duration(a.TimeoutMinutes)*time.Minute + 5*time.Minute),
	}
	if _, err := enqueuer.Enqueue(task, opts...); err != nil {
		return err
	}
	logger.Infof(ctx, "[automation] run enqueued automation=%s trigger=%s", a.ID, trigger)
	return nil
}
