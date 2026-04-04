// internal/kieai/task_manager.go
package kieai

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"goloop/internal/model"
)

// PollTask 表示一个轮询任务
type PollTask struct {
	TaskID    string
	APIKey    string
	SubmitAt  time.Time
	ResultCh  chan *PollResult
	CancelCtx context.Context
}

// PollResult 轮询结果
type PollResult struct {
	Record *model.KieAIRecordData
	Error  error
}

// TaskManager 管理轮询任务的生命周期
type TaskManager struct {
	client      *Client
	cfg         PollerConfig
	taskQueue   chan *PollTask
	activeTasks sync.Map // taskID -> *PollTask
	workerCount int
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewTaskManager 创建任务管理器
func NewTaskManager(client *Client, cfg PollerConfig, workerCount int) *TaskManager {
	if workerCount <= 0 {
		workerCount = 10 // 默认 10 个 worker
	}

	tm := &TaskManager{
		client:      client,
		cfg:         cfg,
		taskQueue:   make(chan *PollTask, workerCount*2),
		workerCount: workerCount,
		stopCh:      make(chan struct{}),
	}

	// 启动 worker 池
	for i := 0; i < workerCount; i++ {
		tm.wg.Add(1)
		go tm.worker(i)
	}

	slog.Info("task manager started", "workers", workerCount)

	return tm
}

// SubmitTask 提交轮询任务（非阻塞）
func (tm *TaskManager) SubmitTask(ctx context.Context, apiKey, taskID string) (*PollResult, error) {
	task := &PollTask{
		TaskID:    taskID,
		APIKey:    apiKey,
		SubmitAt:  time.Now(),
		ResultCh:  make(chan *PollResult, 1),
		CancelCtx: ctx,
	}

	tm.activeTasks.Store(taskID, task)

	select {
	case tm.taskQueue <- task:
		// 任务已入队
	case <-ctx.Done():
		tm.activeTasks.Delete(taskID)
		return nil, ctx.Err()
	}

	// 等待结果
	select {
	case result := <-task.ResultCh:
		tm.activeTasks.Delete(taskID)
		return result, nil
	case <-ctx.Done():
		tm.activeTasks.Delete(taskID)
		return nil, ctx.Err()
	}
}

// worker 执行轮询任务
func (tm *TaskManager) worker(id int) {
	defer tm.wg.Done()

	for {
		select {
		case <-tm.stopCh:
			return
		case task := <-tm.taskQueue:
			tm.pollTask(id, task)
		}
	}
}

func (tm *TaskManager) pollTask(workerID int, task *PollTask) {
	log := slog.With("worker", workerID, "taskId", task.TaskID)
	log.Debug("worker picked up task")

	deadline := task.SubmitAt.Add(tm.cfg.MaxWaitTime)
	interval := tm.cfg.InitialInterval
	consecutiveFails := 0

	for {
		// 检查超时
		if time.Now().After(deadline) {
			task.ResultCh <- &PollResult{
				Error: fmt.Errorf("task timeout after %v", tm.cfg.MaxWaitTime),
			}
			return
		}

		// 检查取消
		select {
		case <-task.CancelCtx.Done():
			task.ResultCh <- &PollResult{Error: task.CancelCtx.Err()}
			return
		case <-time.After(interval):
		}

		// 轮询状态
		record, err := tm.client.GetTaskStatus(task.CancelCtx, task.APIKey, task.TaskID)
		if err != nil {
			consecutiveFails++
			log.Warn("poll failed", "fails", consecutiveFails, "err", err)

			if consecutiveFails >= tm.cfg.RetryAttempts {
				task.ResultCh <- &PollResult{Error: fmt.Errorf("too many failures: %w", err)}
				return
			}
			continue
		}
		consecutiveFails = 0

		// 检查状态
		switch record.State {
		case "success":
			log.Debug("task completed")
			task.ResultCh <- &PollResult{Record: record}
			return

		case "fail":
			reason := record.FailReason
			if reason == "" {
				reason = "unknown failure"
			}
			task.ResultCh <- &PollResult{
				Error: &TaskFailedError{TaskID: task.TaskID, Reason: reason},
			}
			return

		case "waiting", "queuing", "generating":
			// 继续轮询

		default:
			log.Warn("unknown state", "state", record.State)
		}

		// 指数退避
		interval *= 2
		if interval > tm.cfg.MaxInterval {
			interval = tm.cfg.MaxInterval
		}
	}
}

// Stop 停止任务管理器
func (tm *TaskManager) Stop() {
	slog.Info("stopping task manager")
	close(tm.stopCh)
	tm.wg.Wait()
	slog.Info("task manager stopped")
}

// Stats 返回统计信息
func (tm *TaskManager) Stats() map[string]int {
	activeCount := 0
	tm.activeTasks.Range(func(_, _ interface{}) bool {
		activeCount++
		return true
	})

	return map[string]int{
		"workers":      tm.workerCount,
		"active_tasks": activeCount,
		"queue_cap":    cap(tm.taskQueue),
		"queue_len":    len(tm.taskQueue),
	}
}
