package core

import (
    "context"
    "testing"

    gomodel "goloop/internal/model"
)

func TestChannelInterface_SatisfiedByMock(t *testing.T) {
    var _ Channel = &mockChannel{name: "test"}
    ch := &mockChannel{name: "kieai"}

    if ch.Name() != "kieai" { t.Errorf("Name mismatch") }
    if ch.HealthScore() != 1.0 { t.Errorf("HealthScore should be 1.0") }
    if !ch.IsAvailable() { t.Errorf("IsAvailable should be true") }

    resp, err := ch.Generate(context.Background(), "key", &gomodel.GoogleRequest{}, "model")
    if err != nil { t.Fatalf("Generate returned error: %v", err) }
    _ = resp

    id, err := ch.SubmitTask(context.Background(), "key", &gomodel.GoogleRequest{}, "model")
    if err != nil { t.Fatalf("SubmitTask returned error: %v", err) }
    if id != "task-mock" { t.Errorf("SubmitTask taskID mismatch") }

    _, err = ch.PollTask(context.Background(), "key", "task-1")
    if err != nil { t.Fatalf("PollTask returned error: %v", err) }
}

type mockChannel struct{ name string }

func (m *mockChannel) Name() string     { return m.name }
func (m *mockChannel) HealthScore() float64 { return 1.0 }
func (m *mockChannel) IsAvailable() bool  { return true }

func (m *mockChannel) Generate(ctx context.Context, apiKey string, req *gomodel.GoogleRequest, model string) (*gomodel.GoogleResponse, error) {
    return &gomodel.GoogleResponse{
        Candidates: []gomodel.Candidate{
            {Content: gomodel.Content{Parts: []gomodel.Part{{Text: "mock"}}}, FinishReason: "STOP"},
        },
    }, nil
}

func (m *mockChannel) SubmitTask(ctx context.Context, apiKey string, req *gomodel.GoogleRequest, model string) (string, error) {
    return "task-mock", nil
}

func (m *mockChannel) PollTask(ctx context.Context, apiKey, taskID string) (*gomodel.GoogleResponse, error) {
    return &gomodel.GoogleResponse{}, nil
}

func (m *mockChannel) Probe(account Account) bool {
    return true
}