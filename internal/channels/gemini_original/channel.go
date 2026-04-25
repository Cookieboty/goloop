package gemini_original

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"goloop/internal/core"
	"goloop/internal/model"
)

// Channel implements core.Channel, core.RawBodyGenerator, and
// core.RawStreamGenerator for a Google-native Gemini upstream.
// All request and response bytes are forwarded verbatim; the only substitution
// is the base URL and the API key.
//
// Integrating a new Gemini-compatible provider requires:
//  1. Create a *core.DefaultAccountPool and add API keys.
//  2. Call NewChannel with the provider's base URL.
//  3. Register the channel with the PluginRegistry in main.go.
//
// No other files need to change.
type Channel struct {
	core.BaseChannel // provides Name/Weight/IsAvailable/HealthScore/Admin/GetAccountPool
}

// Compile-time assertions.
var _ core.RawBodyGenerator = (*Channel)(nil)
var _ core.RawStreamGenerator = (*Channel)(nil)

// NewChannel creates a Gemini native pass-through channel.
// name is the unique channel identifier (e.g. "gemini-direct").
func NewChannel(name, baseURL string, weight int, pool *core.DefaultAccountPool, timeout time.Duration) *Channel {
	return &Channel{
		BaseChannel: core.NewBaseChannel(name, "gemini_original", baseURL, weight, pool, timeout),
	}
}

// GenerateRaw forwards rawBody verbatim to the upstream Gemini API and returns
// the upstream response bytes unmodified. It implements core.RawBodyGenerator.
func (ch *Channel) GenerateRaw(ctx context.Context, rawBody []byte, modelName string) ([]byte, error) {
	acc, err := ch.Pool.Select()
	if err != nil {
		return nil, fmt.Errorf("gemini: no account available: %w", err)
	}
	acc.IncUsage()

	var success bool
	defer func() { ch.Pool.Return(acc, success) }()

	url := ch.BaseURL + "/v1beta/models/" + modelName + ":generateContent"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Google native API uses x-goog-api-key header for authentication.
	httpReq.Header.Set("x-goog-api-key", acc.APIKey())

	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB limit
	if err != nil {
		return nil, fmt.Errorf("gemini: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, string(data))
	}

	success = true
	return data, nil
}

// StreamRaw opens a streaming request to the upstream Gemini SSE endpoint
// (/v1beta/models/{model}:streamGenerateContent) and pipes the response body
// byte-by-byte to w as it arrives. The upstream response headers are copied
// so the client receives the same Content-Type (text/event-stream).
//
// StreamRaw returns a non-nil error only if the upstream request itself fails
// before any bytes are written. Once streaming has started, errors mid-stream
// are silently dropped (the SSE connection simply closes).
func (ch *Channel) StreamRaw(ctx context.Context, rawBody []byte, modelName string, w core.ResponseWriter) error {
	acc, err := ch.Pool.Select()
	if err != nil {
		return fmt.Errorf("gemini: no account available: %w", err)
	}
	acc.IncUsage()

	var success bool
	defer func() { ch.Pool.Return(acc, success) }()

	url := ch.BaseURL + "/v1beta/models/" + modelName + ":streamGenerateContent?alt=sse"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", acc.APIKey())

	resp, err := ch.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("gemini: stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Mirror upstream response headers to the client.
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(http.StatusOK)

	// Pipe the upstream body directly to the client, flushing after each read.
	buf := make([]byte, 4096)
	success = true
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				// Client disconnected; stop pumping.
				return nil
			}
			w.Flush()
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			// Upstream closed unexpectedly; nothing more to do.
			return nil
		}
	}
}

// Generate is not used for this channel type; the handler calls GenerateRaw
// or StreamRaw instead. This implementation satisfies the core.Channel interface.
func (ch *Channel) Generate(_ context.Context, _ *model.GoogleRequest, _ string) (*model.GoogleResponse, error) {
	return nil, core.ErrNotSupported
}

// Probe performs a lightweight health check by listing available models.
func (ch *Channel) Probe(account core.Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ch.BaseURL+"/v1beta/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("x-goog-api-key", account.APIKey())

	resp, err := ch.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// SubmitTask and PollTask are NOT overridden; BaseChannel returns
// core.ErrNotSupported for both, causing the handler to use GenerateRaw instead.
