// internal/kieai/poller.go
package kieai

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"goloop/internal/model"
)

// PollerConfig holds exponential-backoff parameters.
type PollerConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxWaitTime     time.Duration
	RetryAttempts   int
}

// Poller wraps a Client to poll task status with exponential backoff.
type Poller struct {
	client *Client
	cfg    PollerConfig
}

func NewPoller(client *Client, cfg PollerConfig) *Poller {
	return &Poller{client: client, cfg: cfg}
}

// Poll blocks until the task reaches a terminal state (success/fail) or context is cancelled.
// Returns the completed KieAIRecordData on success.
func (p *Poller) Poll(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
	deadline := time.Now().Add(p.cfg.MaxWaitTime)
	interval := p.cfg.InitialInterval
	pollCount := 0
	consecutiveFails := 0

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("poller: task %q timed out after %v", taskID, p.cfg.MaxWaitTime)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		pollCount++
		record, err := p.client.GetTaskStatus(ctx, apiKey, taskID)
		if err != nil {
			consecutiveFails++
			slog.Warn("poller: poll failed", "taskId", taskID, "attempt", pollCount,
				"consecutiveFails", consecutiveFails, "err", err)
			if consecutiveFails >= p.cfg.RetryAttempts {
				return nil, fmt.Errorf("poller: task %q: %d consecutive failures: %w",
					taskID, consecutiveFails, err)
			}
			// Don't advance interval on error
			continue
		}
		consecutiveFails = 0

		slog.Debug("poller: task status", "taskId", taskID, "state", record.State, "pollCount", pollCount)

		switch record.State {
		case "success":
			return record, nil
		case "fail":
			reason := record.FailReason
			if reason == "" {
				reason = "unknown failure"
			}
			return nil, &TaskFailedError{TaskID: taskID, Reason: reason}
		case "waiting", "queuing", "generating":
			// continue polling
		default:
			slog.Warn("poller: unknown state", "taskId", taskID, "state", record.State)
		}

		// Exponential backoff
		interval *= 2
		if interval > p.cfg.MaxInterval {
			interval = p.cfg.MaxInterval
		}
	}
}

// TaskFailedError is returned when KIE.AI reports the task as failed.
type TaskFailedError struct {
	TaskID string
	Reason string
}

func (e *TaskFailedError) Error() string {
	return fmt.Sprintf("poller: task %q failed: %s", e.TaskID, e.Reason)
}
