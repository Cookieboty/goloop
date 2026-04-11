package kieai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"goloop/internal/model"
)

func TestKIEAIChannel_SubmitTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]string{"taskId": "task-abc"},
		})
	}))
	defer srv.Close()

	pool := NewAccountPool()
	pool.AddAccount("test-key", 100)
	ch := NewChannel(srv.URL, pool, Config{BaseURL: srv.URL})

	if ch.Name() != "kieai" {
		t.Errorf("Name mismatch")
	}
	if !ch.IsAvailable() {
		t.Errorf("IsAvailable should be true")
	}

	taskID, err := ch.SubmitTask(context.Background(), "test-key",
		&model.GoogleRequest{Contents: []model.Content{{Parts: []model.Part{{Text: "test"}}}}},
		"gemini-3.1-flash-image-preview")
	if err != nil {
		t.Fatalf("SubmitTask error: %v", err)
	}
	if taskID != "task-abc" {
		t.Errorf("taskID mismatch: got %q", taskID)
	}

	_ = ch.HealthScore()
}

func TestKIEAIChannel_Probe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user/info" {
			w.Write([]byte(`{"code":200}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	pool := NewAccountPool()
	pool.AddAccount("test-key", 100)
	ch := NewChannel(srv.URL, pool, Config{BaseURL: srv.URL})

	acc := pool.List()[0]
	if !ch.Probe(acc) {
		t.Errorf("Probe should return true when server returns 200")
	}
}