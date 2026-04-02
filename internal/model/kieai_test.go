// internal/model/kieai_test.go
package model

import (
	"encoding/json"
	"testing"
)

func TestKieAIRecordInfoResponse_Success(t *testing.T) {
	raw := `{
        "code": 200,
        "msg": "ok",
        "data": {
            "taskId": "task-123",
            "status": "success",
            "resultJson": {
                "resultUrls": ["https://cdn.example.com/img1.png", "https://cdn.example.com/img2.png"]
            }
        }
    }`
	var resp KieAIRecordInfoResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Data.Status != "success" {
		t.Errorf("status mismatch: %q", resp.Data.Status)
	}
	if resp.Data.ResultJSON == nil {
		t.Fatal("resultJson should not be nil on success")
	}
	if len(resp.Data.ResultJSON.ResultURLs) != 2 {
		t.Errorf("expected 2 result URLs, got %d", len(resp.Data.ResultJSON.ResultURLs))
	}
}

func TestKieAIRecordInfoResponse_InProgress(t *testing.T) {
	raw := `{
        "code": 200,
        "msg": "ok",
        "data": {
            "taskId": "task-456",
            "status": "generating"
        }
    }`
	var resp KieAIRecordInfoResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Data.Status != "generating" {
		t.Errorf("status mismatch: %q", resp.Data.Status)
	}
	if resp.Data.ResultJSON != nil {
		t.Error("resultJson should be nil when in progress")
	}
}

func TestKieAICreateTaskRequest_Marshal(t *testing.T) {
	req := KieAICreateTaskRequest{
		Model: "nano-banana-2",
		Input: KieAIInput{
			Prompt:       "a cat",
			ImageInput:   []string{"https://example.com/ref.png"},
			AspectRatio:  "1:1",
			Resolution:   "1K",
			OutputFormat: "png",
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var check KieAICreateTaskRequest
	if err := json.Unmarshal(b, &check); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}
	if check.Model != "nano-banana-2" {
		t.Errorf("model mismatch: %q", check.Model)
	}
	if len(check.Input.ImageInput) != 1 {
		t.Errorf("image_input length mismatch")
	}
}
