package core

import (
	"context"
	"errors"

	"goloop/internal/model"
)

// ErrNotSupported is returned by Generate, SubmitTask, or PollTask when the
// channel does not support that operation mode. Callers should check with
// errors.Is and fall back to the alternative path.
var ErrNotSupported = errors.New("channel: operation not supported")

// Channel is the interface each AI provider plugin must implement.
type Channel interface {
    Name() string

    // Weight returns the configured weight for weighted random routing.
    Weight() int

    // Generate makes a synchronous call (if provider supports it).
    // Returns error if not supported by this provider.
    Generate(ctx context.Context, req *model.GoogleRequest, model string) (*model.GoogleResponse, error)

    // SubmitTask submits an async task, returns taskID and the account key used.
    // The channel selects an account from its pool internally.
    SubmitTask(ctx context.Context, req *model.GoogleRequest, model string) (taskID string, apiKey string, err error)

    // PollTask retrieves the result of a previously submitted task.
    PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error)

    // HealthScore returns 0.0 (dead) to 1.0 (fully healthy).
    // A channel with HealthScore == 0 is excluded from routing.
    HealthScore() float64

    // IsAvailable returns true if the channel can accept new requests.
    IsAvailable() bool

    // Probe sends a lightweight health probe for a specific account.
    // Returns true if the account responds correctly, false otherwise.
    // Errors are not counted against consecutive failures.
    Probe(account Account) bool
}
