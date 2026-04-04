// internal/kieai/client_test.go
package kieai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"goloop/internal/model"
)

func TestCreateTask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or wrong Authorization header")
		}
		json.NewEncoder(w).Encode(model.KieAICreateTaskResponse{
			Code: 200,
			Data: model.KieAITaskData{TaskID: "task-abc"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 5*time.Second)
	taskID, err := client.CreateTask(context.Background(), "test-key", &model.KieAICreateTaskRequest{
		Model: "nano-banana-2",
		Input: model.KieAIInput{Prompt: "a cat"},
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}
	if taskID != "task-abc" {
		t.Errorf("taskID: got %q, want %q", taskID, "task-abc")
	}
}

func TestCreateTask_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":401,"msg":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 5*time.Second)
	_, err := client.CreateTask(context.Background(), "bad-key", &model.KieAICreateTaskRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	kErr, ok := err.(*ErrKieAI)
	if !ok {
		t.Fatalf("expected *ErrKieAI, got %T", err)
	}
	if kErr.Code != 401 {
		t.Errorf("code: got %d, want 401", kErr.Code)
	}
}

func TestGetTaskStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("taskId") != "task-xyz" {
			t.Errorf("taskId param missing")
		}
		json.NewEncoder(w).Encode(model.KieAIRecordInfoResponse{
			Code: 200,
			Data: model.KieAIRecordData{
				TaskID:        "task-xyz",
				State:         "success",
				ResultJSONRaw: `{"resultUrls":["https://cdn.kie.ai/output/img1.png","https://cdn.kie.ai/output/img2.png"]}`,
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 5*time.Second)
	record, err := client.GetTaskStatus(context.Background(), "test-key", "task-xyz")
	if err != nil {
		t.Fatalf("GetTaskStatus error: %v", err)
	}
	if record.State != "success" {
		t.Errorf("state: got %q", record.State)
	}
	if len(record.ResultJSON().ResultURLs) != 2 {
		t.Errorf("expected 2 result URLs, got %d", len(record.ResultJSON().ResultURLs))
	}
}
