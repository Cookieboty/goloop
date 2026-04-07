// internal/kieai/task_manager_streaming.go
package kieai

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"goloop/internal/model"
)

// SubmitTaskStreaming 非阻塞提交任务，返回结果 channel
// caller 负责从 channel 读取结果
func (tm *TaskManager) SubmitTaskStreaming(ctx context.Context, apiKey, taskID string) <-chan *PollResult {
	resultCh := make(chan *PollResult, 1)

	task := &PollTask{
		TaskID:    taskID,
		APIKey:    apiKey,
		SubmitAt:  time.Now(),
		ResultCh:  resultCh,
		CancelCtx: ctx,
	}

	tm.activeTasks.Store(taskID, task)

	select {
	case tm.taskQueue <- task:
		// 任务已入队
	default:
		// 队列满，返回错误
		resultCh <- &PollResult{Error: fmt.Errorf("task queue full")}
		tm.activeTasks.Delete(taskID)
	}

	return resultCh
}

// PollTaskStreaming 是为 streaming 优化的 poll 实现
// 只在任务完成或失败时返回，不发送中间状态
func (tm *TaskManager) PollTaskStreaming(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
	deadline := time.Now().Add(tm.cfg.MaxWaitTime)
	interval := tm.cfg.InitialInterval
	consecutiveFails := 0

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("task timeout after %v", tm.cfg.MaxWaitTime)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		record, err := tm.client.GetTaskStatus(ctx, apiKey, taskID)
		if err != nil {
			consecutiveFails++
			slog.Warn("poll failed", "taskId", taskID, "fails", consecutiveFails, "err", err)
			if consecutiveFails >= tm.cfg.RetryAttempts {
				return nil, fmt.Errorf("too many failures: %w", err)
			}
			interval *= 2
			if interval > tm.cfg.MaxInterval {
				interval = tm.cfg.MaxInterval
			}
			continue
		}

		consecutiveFails = 0

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
			interval *= 2
			if interval > tm.cfg.MaxInterval {
				interval = tm.cfg.MaxInterval
			}
			// 继续轮询
		}
	}
}