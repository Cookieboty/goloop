package openai_original

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"goloop/internal/core"
	"goloop/internal/model"
)

// Config holds retry settings for transient upstream errors.
type Config struct {
	RetryStatusCodes map[int]struct{}
	RetryAttempts    int
	RetryDelay       time.Duration
}

// Channel implements core.Channel, core.OpenAIRawGenerator, and
// core.OpenAIRawStreamGenerator for OpenAI-compatible upstreams.
// All request and response bytes are forwarded verbatim; the only modifications
// are the base URL and the Authorization header.
//
// Integrating a new OpenAI-compatible provider requires:
//  1. Create a *core.DefaultAccountPool and add API keys.
//  2. Call NewChannel with the provider's base URL.
//  3. Register the channel with the PluginRegistry in main.go.
//
// No other files need to change.
type Channel struct {
	core.BaseChannel // provides Name/Type/Weight/IsAvailable/HealthScore/Admin/GetAccountPool
	cfg              Config
}

// Compile-time assertions.
var _ core.OpenAIRawGenerator = (*Channel)(nil)
var _ core.OpenAIRawStreamGenerator = (*Channel)(nil)

// maxResponseBytes caps non-streaming response bodies. Large image generations
// (b64_json with n=10) may approach 30 MiB, so the limit is generous.
const maxResponseBytes = 64 << 20

// NewChannel creates a gpt-image pass-through channel.
// name is the unique channel identifier (e.g. "gpt-image-primary").
func NewChannel(name, baseURL string, weight int, pool *core.DefaultAccountPool, timeout time.Duration, cfg Config) *Channel {
	return &Channel{
		BaseChannel: core.NewBaseChannel(name, "openai_original", baseURL, weight, pool, timeout),
		cfg:         cfg,
	}
}

// GenerateOpenAIRaw forwards rawBody verbatim to the upstream OpenAI-compatible API and
// returns the upstream status/headers/body unmodified. Non-2xx responses are returned
// in the result (not as an error) so the handler can decide whether to fall back to
// another channel or propagate the upstream error to the client directly.
//
// If the upstream returns a status in RetryStatusCodes, the request is retried
// up to RetryAttempts times with RetryDelay between attempts.
func (ch *Channel) GenerateOpenAIRaw(ctx context.Context, contentType string, rawBody []byte, endpoint string) (*core.OpenAIRawResponse, error) {
	acc, err := ch.Pool.Select()
	if err != nil {
		return nil, fmt.Errorf("gpt-image: no account available: %w", err)
	}
	acc.IncUsage()

	var success bool
	defer func() { ch.Pool.Return(acc, success) }()

	if contentType == "" {
		contentType = "application/json"
	}

	url := ch.BaseURL + endpoint

	var lastStatus int
	var lastHeaders http.Header
	var lastBody []byte
	var ttfbMs, bodyReadMs int64

	for attempt := 0; attempt <= ch.cfg.RetryAttempts; attempt++ {
		if attempt > 0 {
			slog.Warn("retrying on retriable status",
				"channel", ch.Name(), "endpoint", endpoint,
				"status", lastStatus, "attempt", attempt,
				"maxRetries", ch.cfg.RetryAttempts)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(ch.cfg.RetryDelay):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", contentType)
		httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())

		start := time.Now()
		resp, err := ch.HTTPClient.Do(httpReq)
		ttfbMs = time.Since(start).Milliseconds()
		if err != nil {
			return nil, fmt.Errorf("gpt-image: HTTP request failed: %w", err)
		}

		bodyStart := time.Now()
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		bodyReadMs = time.Since(bodyStart).Milliseconds()
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("gpt-image: read response: %w", readErr)
		}

		lastStatus = resp.StatusCode
		lastHeaders = resp.Header.Clone()
		lastBody = data

		if !ch.isRetryable(resp.StatusCode) {
			break
		}
	}

	if lastStatus >= 200 && lastStatus < 300 {
		success = true
	}

	return &core.OpenAIRawResponse{
		Status:        lastStatus,
		Headers:       lastHeaders,
		Body:          lastBody,
		TTFBMs:        ttfbMs,
		BodyReadMs:    bodyReadMs,
		ResponseBytes: len(lastBody),
	}, nil
}

// StreamOpenAIRaw forwards rawBody verbatim to the upstream OpenAI-compatible API and
// streams the response back to the client. See OpenAIRawStreamGenerator docs for the
// contract: pre-commit errors (transport or non-2xx upstream) return an error so the
// handler can fall back; post-commit errors are swallowed to avoid corrupting the
// already-started client response.
//
// If the upstream returns a retriable status (pre-commit), the request is retried
// up to RetryAttempts times with RetryDelay between attempts.
func (ch *Channel) StreamOpenAIRaw(ctx context.Context, contentType string, rawBody []byte, endpoint string, w core.ResponseWriter) error {
	acc, err := ch.Pool.Select()
	if err != nil {
		return fmt.Errorf("gpt-image: no account available: %w", err)
	}
	acc.IncUsage()

	var success bool
	defer func() { ch.Pool.Return(acc, success) }()

	if contentType == "" {
		contentType = "application/json"
	}

	url := ch.BaseURL + endpoint

	// Pre-commit retry loop: retries only happen before any bytes reach the client.
	var resp *http.Response
	for attempt := 0; attempt <= ch.cfg.RetryAttempts; attempt++ {
		if attempt > 0 {
			slog.Warn("retrying on retriable status (stream)",
				"channel", ch.Name(), "endpoint", endpoint,
				"status", resp.StatusCode, "attempt", attempt,
				"maxRetries", ch.cfg.RetryAttempts)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(ch.cfg.RetryDelay):
			}
		}

		httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
		if reqErr != nil {
			return reqErr
		}
		httpReq.Header.Set("Content-Type", contentType)
		httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())

		resp, err = ch.HTTPClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("gpt-image: HTTP request failed: %w", err)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}

		if !ch.isRetryable(resp.StatusCode) {
			break
		}

		// Drain body before retry so the connection can be reused.
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
	}
	defer resp.Body.Close()

	// Pre-commit error: non-2xx → let the handler fall back.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return &UpstreamStatusError{
			Status:  resp.StatusCode,
			Headers: resp.Header.Clone(),
			Body:    body,
		}
	}

	// Mirror a safe subset of upstream headers. Do NOT copy Content-Length
	// or Transfer-Encoding — net/http handles framing itself and copying
	// them leads to chunking corruption.
	mirrorHeaders(w.Header(), resp.Header)
	w.WriteHeader(http.StatusOK)

	// Headers are now committed. From this point on we must not return an
	// error, because the handler would try the next channel and double-write.
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				success = true
				return nil
			}
			w.Flush()
		}
		if readErr == io.EOF {
			success = true
			return nil
		}
		if readErr != nil {
			success = true
			return nil
		}
	}
}

func (ch *Channel) isRetryable(statusCode int) bool {
	_, ok := ch.cfg.RetryStatusCodes[statusCode]
	return ok && ch.cfg.RetryAttempts > 0
}

// UpstreamStatusError is returned by StreamOpenAIRaw when the upstream returns
// a non-2xx status BEFORE any bytes are written to the client. Handlers can
// type-assert on this error to decide whether to fall back to another channel
// (for 5xx/429/408/401) or propagate the status/body directly to the client
// (for other 4xx like 400/403/404/422).
type UpstreamStatusError struct {
	Status  int
	Headers http.Header
	Body    []byte
}

func (e *UpstreamStatusError) Error() string {
	return fmt.Sprintf("gpt-image: upstream HTTP %d", e.Status)
}

// headersToCopy is the whitelist of upstream response headers safe to mirror
// to the client. Framing-related headers (Content-Length, Transfer-Encoding,
// Connection) are intentionally excluded because net/http manages them.
var headersToCopy = map[string]struct{}{
	"Content-Type":         {},
	"Cache-Control":        {},
	"X-Request-Id":         {},
	"Openai-Version":       {},
	"Openai-Processing-Ms": {},
	"X-Ratelimit-Limit-Requests":     {},
	"X-Ratelimit-Limit-Tokens":       {},
	"X-Ratelimit-Remaining-Requests": {},
	"X-Ratelimit-Remaining-Tokens":   {},
	"X-Ratelimit-Reset-Requests":     {},
	"X-Ratelimit-Reset-Tokens":       {},
	"Retry-After":                    {},
}

// mirrorHeaders copies whitelisted headers from src to dst. Header names are
// canonicalized by http.Header so map lookup works with the canonical form.
func mirrorHeaders(dst, src http.Header) {
	for k, vs := range src {
		if _, ok := headersToCopy[k]; !ok {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// Generate returns ErrNotSupported. This channel only supports raw pass-through.
func (ch *Channel) Generate(_ context.Context, _ *model.GoogleRequest, _ string) (*model.GoogleResponse, error) {
	return nil, core.ErrNotSupported
}

// Probe sends a lightweight health probe to the upstream API.
// It calls GET /v1/models to verify the account is valid.
func (ch *Channel) Probe(account core.Account) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ch.BaseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+account.APIKey())

	resp, err := ch.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// SubmitTask and PollTask are NOT overridden; BaseChannel returns
// core.ErrNotSupported for both, causing the handler to use GenerateOpenAIRaw instead.
