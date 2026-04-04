// internal/kieai/poller_test.go
package kieai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"goloop/internal/model"
)

func newTestPoller(serverURL string) *Poller {
	client := NewClient(serverURL, 5*time.Second)
	return NewPoller(client, PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		MaxWaitTime:     5 * time.Second,
		RetryAttempts:   3,
	})
}

func TestPoller_SuccessAfterQueuing(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		var resp model.KieAIRecordInfoResponse
		if n < 3 {
			resp = model.KieAIRecordInfoResponse{
				Data: model.KieAIRecordData{State: "queuing"},
			}
		} else {
			resp = model.KieAIRecordInfoResponse{
				Data: model.KieAIRecordData{
					State:         "success",
					ResultJSONRaw: `{"resultUrls":["https://cdn.kie.ai/img.png"]}`,
				},
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	poller := newTestPoller(srv.URL)
	record, err := poller.Poll(context.Background(), "test-key", "task-1")
	if err != nil {
		t.Fatalf("Poll error: %v", err)
	}
	if record.State != "success" {
		t.Errorf("expected success, got %q", record.State)
	}
	if callCount.Load() < 3 {
		t.Errorf("expected at least 3 polls, got %d", callCount.Load())
	}
}

func TestPoller_TaskFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(model.KieAIRecordInfoResponse{
			Data: model.KieAIRecordData{
				State:      "fail",
				FailReason: "content policy violation",
			},
		})
	}))
	defer srv.Close()

	poller := newTestPoller(srv.URL)
	_, err := poller.Poll(context.Background(), "test-key", "task-2")
	if err == nil {
		t.Fatal("expected error for failed task")
	}
	tErr, ok := err.(*TaskFailedError)
	if !ok {
		t.Fatalf("expected *TaskFailedError, got %T: %v", err, err)
	}
	if tErr.Reason != "content policy violation" {
		t.Errorf("reason mismatch: %q", tErr.Reason)
	}
}

func TestPoller_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(model.KieAIRecordInfoResponse{
			Data: model.KieAIRecordData{State: "generating"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 5*time.Second)
	poller := NewPoller(client, PollerConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     20 * time.Millisecond,
		MaxWaitTime:     100 * time.Millisecond,
		RetryAttempts:   3,
	})

	_, err := poller.Poll(context.Background(), "test-key", "task-3")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestPoller_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(model.KieAIRecordInfoResponse{
			Data: model.KieAIRecordData{State: "waiting"},
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	poller := newTestPoller(srv.URL)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := poller.Poll(ctx, "test-key", "task-4")
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}
