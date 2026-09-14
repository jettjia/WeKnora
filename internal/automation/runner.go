package automation

// Runner executes one automation run inside an Asynq worker: single-flight
// lock → fresh session + messages → eventBus capture → AgentQA → status
// recording. Mirrors the IM channel's programmatic AgentQA invocation
// (internal/im/service.go) — no HTTP involved.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// runTaskPayload is the JSON body of an automation:run task.
type runTaskPayload struct {
	AutomationID string `json:"automation_id"`
	TenantID     uint64 `json:"tenant_id"`
	Trigger      string `json:"trigger"`
}

// Runner executes automation runs.
type Runner struct {
	repo     *Repository
	sessions interfaces.SessionService
	messages interfaces.MessageService
	agents   interfaces.CustomAgentService
	redis    *redis.Client
}

// NewRunner builds the run executor.
func NewRunner(
	repo *Repository,
	sessions interfaces.SessionService,
	messages interfaces.MessageService,
	agents interfaces.CustomAgentService,
	redisClient *redis.Client,
) *Runner {
	return &Runner{
		repo: repo, sessions: sessions, messages: messages,
		agents: agents, redis: redisClient,
	}
}

// HandleRunTask is the Asynq handler for automation:run. It never returns a
// retryable error (MaxRetry is 0 by design): the automation_runs row is the
// source of truth for outcomes.
func (r *Runner) HandleRunTask(ctx context.Context, t *asynq.Task) error {
	var p runTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		logger.Errorf(ctx, "[automation] bad task payload: %v", err)
		return nil
	}
	a, err := r.repo.FindAutomationAnyTenant(ctx, p.AutomationID)
	if err != nil {
		logger.Warnf(ctx, "[automation] task for missing automation=%s: %v", p.AutomationID, err)
		return nil
	}
	timeout := time.Duration(a.TimeoutMinutes) * time.Minute
	lockTTL := timeout + 5*time.Minute
	lockKey := fmt.Sprintf("automation:lock:%s", a.ID)

	// ---- single-flight ----
	acquired, err := r.acquireLock(ctx, lockKey, lockTTL)
	if err != nil {
		logger.Errorf(ctx, "[automation] lock check failed automation=%s: %v", a.ID, err)
		return nil
	}
	if !acquired {
		// Previous run still holds the lock.
		if a.OverlapPolicy == OverlapQueue {
			// Queue: poll for the lock within the timeout budget.
			deadline := time.Now().Add(timeout)
			for time.Now().Before(deadline) {
				time.Sleep(10 * time.Second)
				if acquired, _ = r.acquireLock(ctx, lockKey, lockTTL); acquired {
					break
				}
			}
		}
		if !acquired {
			logger.Infof(ctx, "[automation] overlap skip automation=%s policy=%s", a.ID, a.OverlapPolicy)
			now := time.Now().UTC()
			_ = r.repo.CreateRun(ctx, &AutomationRun{
				TenantID: a.TenantID, AutomationID: a.ID,
				Status: RunStatusSkipped, TriggerType: p.Trigger,
				StartedAt: &now, FinishedAt: &now,
				OutputSummary: "skipped: previous run still in progress",
			})
			return nil
		}
	}
	defer func() { _ = r.redis.Del(context.Background(), lockKey) }()

	// ---- execute ----
	run := &AutomationRun{
		TenantID: a.TenantID, AutomationID: a.ID,
		Status: RunStatusRunning, TriggerType: p.Trigger,
	}
	now := time.Now().UTC()
	run.StartedAt = &now
	if err := r.repo.CreateRun(ctx, run); err != nil {
		logger.Errorf(ctx, "[automation] create run row failed: %v", err)
		return nil
	}

	status, output, runErr := r.executeRun(ctx, a, run, timeout)

	finished := time.Now().UTC()
	run.FinishedAt = &finished
	run.DurationMs = finished.Sub(now).Milliseconds()
	run.Status = status
	run.OutputSummary = truncateRunes(output, 500)
	if runErr != nil {
		run.Error = runErr.Error()
	}
	if err := r.repo.UpdateRun(ctx, run); err != nil {
		logger.Errorf(ctx, "[automation] update run failed run=%s: %v", run.ID, err)
	}
	logger.Infof(ctx, "[automation] run finished run=%s automation=%s status=%s duration=%dms",
		run.ID, a.ID, status, run.DurationMs)
	return nil
}

// executeRun drives one full AgentQA pass and returns (status, output, err).
func (r *Runner) executeRun(
	ctx context.Context,
	a *Automation,
	run *AutomationRun,
	timeout time.Duration,
) (string, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The run acts as the automation creator's identity: agent permission
	// checks, KB scope and tool identity all resolve from this context.
	runCtx = context.WithValue(runCtx, types.TenantIDContextKey, a.TenantID)
	runCtx = context.WithValue(runCtx, types.UserIDContextKey, a.CreatedBy)

	// Load the bound agent (tenant comes from the context above).
	agent, err := r.agents.GetAgentByID(runCtx, a.AgentID)
	if err != nil || agent == nil {
		return RunStatusFailed, "", fmt.Errorf("load agent %s: %w", a.AgentID, err)
	}

	// Fresh session per run (design decision: no cross-run context).
	session, err := r.sessions.CreateSession(runCtx, &types.Session{
		TenantID:    a.TenantID,
		Title:       fmt.Sprintf("%s · %s", a.Title, time.Now().In(time.Local).Format("01-02 15:04")),
		Description: "Automation run: " + a.Name,
	})
	if err != nil {
		return RunStatusFailed, "", fmt.Errorf("create session: %w", err)
	}
	run.SessionID = session.ID
	_ = r.repo.UpdateRun(ctx, run)

	query := RenderTemplate(a.QueryTemplate, time.Now())
	requestID := run.ID

	userMsg, err := r.messages.CreateMessage(runCtx, &types.Message{
		SessionID: session.ID, Role: "user", Content: query,
		RequestID: requestID, CreatedAt: time.Now(), IsCompleted: true,
		Channel: "automation",
	})
	if err != nil {
		return RunStatusFailed, "", fmt.Errorf("create user message: %w", err)
	}
	assistantMsg, err := r.messages.CreateMessage(runCtx, &types.Message{
		SessionID: session.ID, Role: "assistant",
		RequestID: requestID, CreatedAt: time.Now(), IsCompleted: false,
		Channel: "automation",
	})
	if err != nil {
		return RunStatusFailed, "", fmt.Errorf("create assistant message: %w", err)
	}

	// Capture the final answer through the event bus (same contract the IM
	// channel uses): FinalAnswer deltas accumulate until Done.
	var buf strings.Builder
	var evtErr string
	eventBus := event.NewEventBus()
	eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		if !ok {
			return nil
		}
		buf.WriteString(data.Content)
		return nil
	})
	eventBus.On(event.EventError, func(_ context.Context, evt event.Event) error {
		if data, ok := evt.Data.(event.ErrorData); ok && evtErr == "" {
			evtErr = data.Error
		}
		return nil
	})

	req := &types.QARequest{
		Session:            session,
		Query:              query,
		AssistantMessageID: assistantMsg.ID,
		UserMessageID:      userMsg.ID,
		CustomAgent:        agent,
		WebSearchEnabled:   agent.Config.WebSearchEnabled,
	}
	qaErr := r.sessions.AgentQA(runCtx, req, eventBus)

	final := strings.TrimSpace(buf.String())
	if qaErr != nil {
		status := RunStatusFailed
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			status = RunStatusTimeout
			qaErr = fmt.Errorf("run exceeded timeout of %s", timeout)
		}
		// Persist the failure on the assistant message so session replay
		// shows what happened.
		assistantMsg.Content = "Automation run failed: " + errString(qaErr)
		assistantMsg.IsCompleted = true
		_ = r.messages.UpdateMessage(context.WithoutCancel(ctx), assistantMsg)
		return status, final, qaErr
	}
	if evtErr != "" && final == "" {
		assistantMsg.Content = "Automation run failed: " + evtErr
		assistantMsg.IsCompleted = true
		_ = r.messages.UpdateMessage(context.WithoutCancel(ctx), assistantMsg)
		return RunStatusFailed, "", errors.New(evtErr)
	}

	assistantMsg.Content = final
	assistantMsg.IsCompleted = true
	if err := r.messages.UpdateMessage(context.WithoutCancel(ctx), assistantMsg); err != nil {
		logger.Warnf(ctx, "[automation] finalize assistant message failed: %v", err)
	}
	return RunStatusSuccess, final, nil
}

// acquireLock tries one SetNX round.
func (r *Runner) acquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := r.redis.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func truncateRunes(s string, n int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "..."
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
