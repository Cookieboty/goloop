package core

import (
	"context"
	"errors"
	"net/http"

	"goloop/internal/model"
)

// ErrNotSupported is returned by Generate, SubmitTask, or PollTask when the
// channel does not support that operation mode. Callers should check with
// errors.Is and fall back to the alternative path.
var ErrNotSupported = errors.New("channel: operation not supported")

// RawBodyGenerator is an optional interface for channels that perform
// zero-conversion pass-through to a Google-native upstream. When a channel
// implements this interface, the handler will call GenerateRaw with the
// unmodified request body bytes instead of the parsed struct, and will write
// the raw response bytes directly back to the client.
type RawBodyGenerator interface {
	GenerateRaw(ctx context.Context, rawBody []byte, modelName string) ([]byte, error)
}

// RawStreamGenerator is an optional interface for channels that can stream
// a Google-native SSE response verbatim to the client. The implementation
// must write SSE events directly to w and flush as data arrives.
// Returning a non-nil error means the upstream request itself failed before
// any bytes were written (caller may fall back or report error).
type RawStreamGenerator interface {
	StreamRaw(ctx context.Context, rawBody []byte, modelName string, w ResponseWriter) error
}

// StreamGenerator is an optional interface for channels that can produce a
// streaming response in Google SSE format even when the upstream protocol
// differs (e.g. OpenAI SSE). The implementation translates each upstream
// event and writes Google-format SSE events to w.
type StreamGenerator interface {
	Stream(ctx context.Context, req *model.GoogleRequest, modelName string, w ResponseWriter) error
}

// ResponseWriter is the subset of http.ResponseWriter used by streaming
// channel implementations so they can be tested without a real HTTP server.
type ResponseWriter interface {
	Header() http.Header
	Write([]byte) (int, error)
	WriteHeader(statusCode int)
	Flush()
}

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
