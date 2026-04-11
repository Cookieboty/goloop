package core

import (
    "context"

    "goloop/internal/model"
)

// Account represents an AI provider account with its credentials and state.
type Account interface {
    APIKey() string
    Weight() int
    UsageCount() int
    HealthScore() float64
    IsHealthy() bool
    IncUsage()
    RecordFailure()
    RecordSuccess()
}

// Channel is the interface each AI provider plugin must implement.
type Channel interface {
    Name() string

    // Generate makes a synchronous call (if provider supports it).
    // Returns error if not supported by this provider.
    Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (*model.GoogleResponse, error)

    // SubmitTask submits an async task, returns taskID.
    SubmitTask(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (string, error)

    // PollTask retrieves the result of a previously submitted task.
    PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error)

    // HealthScore returns 0.0 (dead) to 1.0 (fully healthy).
    HealthScore() float64

    // IsAvailable returns true if the channel can accept new requests.
    IsAvailable() bool

    // Probe sends a lightweight health probe for a specific account.
    // Returns true if the account responds correctly, false otherwise.
    // Errors are not counted against consecutive failures.
    Probe(account Account) bool
}