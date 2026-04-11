# Multi-Channel Gateway Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a microkernel-style gateway that self-issues JWTs for auth, routes requests across multiple AI provider channels (starting with KIE.AI), rotates across multiple accounts per channel with weighted random selection, and guarantees Gemini-compatible input/output format at the API surface.

**Architecture:** Microkernel with plugin-based channels. Core handles JWT lifecycle, routing policy, and request orchestration. Each AI provider (KIE.AI, Gemini Direct, etc.) is a isolated plugin managing its own account pool, transformation, and health tracking. Plugins communicate through a well-defined `Channel` interface. The core knows nothing about specific provider APIs.

**Tech Stack:** Go 1.23, `github.com/golang-jwt/jwt/v5`, `gopkg.in/yaml.v3`, standard `net/http`, `log/slog`

---

## File Map

```
New files to CREATE:
  internal/core/channel.go          — Channel plugin interface
  internal/core/pluginRegistry.go   — Plugin registry and lifecycle
  internal/core/router.go           — Weighted random + health-aware routing
  internal/core/jwt.go              — JWT issuer and validator
  internal/core/request.go          — Core request context
  internal/core/health.go           — Channel/account health tracker
  internal/channels/kieai/channel.go — KIE.AI channel plugin
  internal/channels/kieai/accountPool.go — Per-channel account pool with weighted random
  internal/channels/kieai/requestTransformer.go — KIE.AI-specific request transform
  internal/channels/kieai/responseTransformer.go — KIE.AI-specific response transform
  internal/channels/kieai/accountPool_test.go
  internal/channels/kieai/channel_test.go
  internal/core/jwt_test.go
  internal/core/router_test.go
  internal/core/pluginRegistry_test.go
  internal/core/health_test.go
  cmd/server/main.go               — Rewritten to wire core + plugins

Files to MODIFY:
  internal/handler/gemini_handler.go — Refactor: strip KIE-specific code, delegate to core
  internal/config/config.go         — Add multi-channel + multi-account config
  config/config.yaml                — Add channels/accounts config structure
  go.mod                            — Add jwt dependency
  go.sum                            — Will be updated by go mod tidy

Files to PRESERVE (reference for migration):
  internal/kieai/client.go          — Migrate logic into kieai/channel.go
  internal/kieai/task_manager.go    — Migrate logic into kieai/channel.go
  internal/kieai/task_manager_streaming.go
  internal/transformer/request_transformer.go — Migrate into kieai/requestTransformer.go
  internal/transformer/response_transformer.go — Migrate into kieai/responseTransformer.go
```

---

## Chunk 1: Core Interfaces and Plugin System

### Task 1: Core — Channel Plugin Interface

**Files:**
- Create: `internal/core/channel.go`
- Test: `internal/core/channel_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/channel_test.go
package core

import (
    "context"
    "testing"

    "goloop/internal/model"
)

func TestChannelInterface(t *testing.T) {
    // Verify Channel interface is satisfied by a mock
    var _ Channel = &mockChannel{}

    req := &model.GoogleRequest{
        Contents: []model.Content{
            {Parts: []model.Part{{Text: "test"}}},
        },
    }

    mock := &mockChannel{name: "test"}
    resp, err := mock.Generate(context.Background(), "apiKey", req, "gemini-model")
    if err != nil {
        t.Fatalf("mockChannel should implement Channel: %v", err)
    }
    _ = resp
}

type mockChannel struct {
    name string
}

func (m *mockChannel) Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (*model.GoogleResponse, error) {
    return &model.GoogleResponse{
        Candidates: []model.Candidate{
            {Content: model.Content{Parts: []model.Part{{Text: "mock"}}}, FinishReason: "STOP"},
        },
    }, nil
}

func (m *mockChannel) Name() string                            { return m.name }
func (m *mockChannel) HealthScore() float64                   { return 1.0 }
func (m *mockChannel) SubmitTask(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (string, error) {
    return "mock-task-id", nil
}
func (m *mockChannel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
    return &model.GoogleResponse{}, nil
}
func (m *mockChannel) IsAvailable() bool                    { return true }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/channel_test.go -v`
Expected: FAIL — "未定义 Channel 类型"

- [ ] **Step 3: Write minimal interface**

```go
// internal/core/channel.go
package core

import (
    "context"

    "goloop/internal/model"
)

// Channel is the interface each AI provider plugin must implement.
// The core orchestrates routing and account selection; the channel
// plugin handles provider-specific API calls, auth, and transformation.
type Channel interface {
    // Name returns the unique channel identifier (e.g. "kieai", "gemini-direct").
    Name() string

    // Generate makes a synchronous (blocking) image generation call.
    // The apiKey is the credential selected by the account pool.
    Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (*model.GoogleResponse, error)

    // SubmitTask submits an async task and returns a task ID.
    // Used for long-running operations that need polling.
    SubmitTask(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (string, error)

    // PollTask retrieves the result of a previously submitted task.
    PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error)

    // HealthScore returns a score from 0.0 (dead) to 1.0 (fully healthy).
    // Used by the router for weighted selection.
    HealthScore() float64

    // IsAvailable returns true if the channel can accept new requests.
    IsAvailable() bool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/channel_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/channel.go internal/core/channel_test.go
git commit -m "feat(core): add Channel plugin interface"
```

---

### Task 2: Core — Account and AccountPool Interface

**Files:**
- Create: `internal/core/account.go`
- Test: `internal/core/account_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/account_test.go
package core

import "testing"

func TestAccountInterface(t *testing.T) {
    var _ Account = &mockAccount{}

    acc := &mockAccount{key: "test-key", weight: 50}
    if acc.APIKey() != "test-key" {
        t.Errorf("APIKey mismatch")
    }
    if acc.Weight() != 50 {
        t.Errorf("Weight mismatch")
    }
    acc.IncUsage()
    if acc.UsageCount() != 1 {
        t.Errorf("UsageCount should be 1")
    }
    acc.RecordFailure()
    if acc.HealthScore() >= 1.0 {
        t.Errorf("HealthScore should decrease after failure")
    }
}

type mockAccount struct {
    key        string
    weight     int
    usageCount int
    failCount  int
}

func (m *mockAccount) APIKey() string      { return m.key }
func (m *mockAccount) Weight() int         { return m.weight }
func (m *mockAccount) UsageCount() int     { return m.usageCount }
func (m *mockAccount) HealthScore() float64 { return 1.0 }

func (m *mockAccount) IncUsage()           { m.usageCount++ }
func (m *mockAccount) RecordFailure()       { m.failCount++ }
func (m *mockAccount) RecordSuccess()      { m.failCount = max(0, m.failCount-1) }
func (m *mockAccount) IsHealthy() bool      { return m.failCount < 3 }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/account_test.go -v`
Expected: FAIL — "未定义 Account 类型"

- [ ] **Step 3: Write minimal interface**

```go
// internal/core/account.go
package core

// Account represents a single API credential within a channel.
// Each account has a weight (for weighted random selection) and
// health tracking (for failover).
type Account interface {
    // APIKey returns the raw API key or token for this account.
    APIKey() string

    // Weight returns the selection weight (higher = more likely to be selected).
    // Weight must be > 0.
    Weight() int

    // UsageCount returns how many requests have been assigned to this account.
    UsageCount() int

    // HealthScore returns a score from 0.0 to 1.0. Below 0.5 means unhealthy.
    HealthScore() float64

    // IsHealthy returns true if the account can be used.
    IsHealthy() bool

    // IncUsage increments the usage counter.
    IncUsage()

    // RecordFailure increments the failure counter.
    RecordFailure()

    // RecordSuccess decrements the failure counter (minimum 0).
    RecordSuccess()
}

// AccountPool selects accounts using weighted random with health awareness.
type AccountPool interface {
    // Select returns an account based on weighted random selection,
    // excluding accounts that are unhealthy or at zero weight.
    Select() (Account, error)

    // Return returns an account back to the pool after use (with result).
    Return(account Account, success bool)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/account_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/account.go internal/core/account_test.go
git commit -m "feat(core): add Account and AccountPool interfaces"
```

---

### Task 3: Core — Plugin Registry

**Files:**
- Create: `internal/core/pluginRegistry.go`
- Test: `internal/core/pluginRegistry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/pluginRegistry_test.go
package core

import (
    "testing"
)

func TestPluginRegistry(t *testing.T) {
    reg := NewPluginRegistry()

    mock := &mockChannel{name: "test-channel"}
    reg.Register(mock)

    ch, err := reg.Get("test-channel")
    if err != nil {
        t.Fatalf("expected to get channel: %v", err)
    }
    if ch.Name() != "test-channel" {
        t.Errorf("name mismatch: got %q", ch.Name())
    }

    _, err = reg.Get("nonexistent")
    if err == nil {
        t.Errorf("expected error for nonexistent channel")
    }

    channels := reg.List()
    if len(channels) != 1 {
        t.Errorf("expected 1 channel, got %d", len(channels))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/pluginRegistry_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/core/pluginRegistry.go
package core

import (
    "fmt"
    "sync"
)

// PluginRegistry holds all registered Channel plugins.
type PluginRegistry struct {
    mu       sync.RWMutex
    channels map[string]Channel
}

// NewPluginRegistry creates an empty registry.
func NewPluginRegistry() *PluginRegistry {
    return &PluginRegistry{
        channels: make(map[string]Channel),
    }
}

// Register adds a channel to the registry. Panics if a channel with the same name already exists.
func (r *PluginRegistry) Register(ch Channel) {
    r.mu.Lock()
    defer r.mu.Unlock()

    if ch == nil {
        panic("cannot register nil channel")
    }
    name := ch.Name()
    if name == "" {
        panic("channel name cannot be empty")
    }
    if _, exists := r.channels[name]; exists {
        panic(fmt.Sprintf("channel %q already registered", name))
    }
    r.channels[name] = ch
}

// Get returns a channel by name. Returns an error if not found.
func (r *PluginRegistry) Get(name string) (Channel, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    ch, ok := r.channels[name]
    if !ok {
        return nil, fmt.Errorf("pluginRegistry: channel %q not found", name)
    }
    return ch, nil
}

// List returns all registered channels.
func (r *PluginRegistry) List() []Channel {
    r.mu.RLock()
    defer r.mu.RUnlock()

    ret := make([]Channel, 0, len(r.channels))
    for _, ch := range r.channels {
        ret = append(ret, ch)
    }
    return ret
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/pluginRegistry_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/pluginRegistry.go internal/core/pluginRegistry_test.go
git commit -m "feat(core): add PluginRegistry for channel lifecycle management"
```

---

### Task 4: Core — Health Tracker

**Files:**
- Create: `internal/core/health.go`
- Test: `internal/core/health_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/health_test.go
package core

import (
    "testing"
    "time"
)

func TestHealthTracker(t *testing.T) {
    ht := NewHealthTracker()

    // Initial state: healthy
    if !ht.IsHealthy("kieai") {
        t.Errorf("expected healthy initially")
    }

    // Record failures
    ht.RecordFailure("kieai")
    ht.RecordFailure("kieai")
    if ht.HealthScore("kieai") >= 0.5 {
        t.Errorf("health should decrease after failures")
    }

    // Record success
    ht.RecordSuccess("kieai")
    if ht.HealthScore("kieai") < 0.5 {
        t.Errorf("health should improve after success")
    }

    // RecordLatency
    ht.RecordLatency("kieai", 200*time.Millisecond)
    ht.RecordLatency("kieai", 300*time.Millisecond)
    avg := ht.AverageLatency("kieai")
    if avg < 200*time.Millisecond || avg > 300*time.Millisecond {
        t.Errorf("average latency out of range: %v", avg)
    }

    // Unhealthy channel
    for i := 0; i < 5; i++ {
        ht.RecordFailure("kieai")
    }
    if ht.IsHealthy("kieai") {
        t.Errorf("channel should be unhealthy after 5 consecutive failures")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/health_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/core/health.go
package core

import (
    "math"
    "sync"
    "time"
)

const (
    failureDecay   = 0.2   // how much each failure decrements health
    successRecovery = 0.1   // how much each success increments health
    minHealth      = 0.0
    maxHealth      = 1.0
    failureThreshold = 5    // consecutive failures before marked unhealthy
)

// HealthTracker records success/failure/latency per channel and computes health scores.
type HealthTracker struct {
    mu          sync.RWMutex
    consecutive  map[string]int        // consecutive failure count
    totalFail    map[string]int        // total failures
    totalSuccess map[string]int        // total successes
    latencies    map[string][]time.Duration
    health       map[string]float64   // computed health scores
}

// NewHealthTracker creates a fresh tracker with all channels starting at full health.
func NewHealthTracker() *HealthTracker {
    return &HealthTracker{
        consecutive:  make(map[string]int),
        totalFail:    make(map[string]int),
        totalSuccess: make(map[string]int),
        latencies:    make(map[string][]time.Duration),
        health:       make(map[string]float64),
    }
}

// RecordFailure increments the failure counter for a channel.
func (h *HealthTracker) RecordFailure(channel string) {
    h.mu.Lock()
    defer h.mu.Unlock()

    h.consecutive[channel]++
    h.totalFail[channel]++
    h.recalc(channel)
}

// RecordSuccess increments the success counter and resets consecutive failures.
func (h *HealthTracker) RecordSuccess(channel string) {
    h.mu.Lock()
    defer h.mu.Unlock()

    h.consecutive[channel] = 0
    h.totalSuccess[channel]++
    h.recalc(channel)
}

// RecordLatency records a latency sample for a channel.
func (h *HealthTracker) RecordLatency(channel string, d time.Duration) {
    h.mu.Lock()
    defer h.mu.Unlock()

    h.latencies[channel] = append(h.latencies[channel], d)
    // keep only last 100 samples
    if len(h.latencies[channel]) > 100 {
        h.latencies[channel] = h.latencies[channel][1:]
    }
}

// HealthScore returns the computed health score (0.0 to 1.0) for a channel.
func (h *HealthTracker) HealthScore(channel string) float64 {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return h.health[channel]
}

// AverageLatency returns the rolling average latency for a channel.
func (h *HealthTracker) AverageLatency(channel string) time.Duration {
    h.mu.RLock()
    defer h.mu.RUnlock()

    lats := h.latencies[channel]
    if len(lats) == 0 {
        return 0
    }
    var sum int64
    for _, d := range lats {
        sum += int64(d)
    }
    return time.Duration(sum / int64(len(lats)))
}

// IsHealthy returns true if the channel's health score is >= 0.5 and consecutive failures < threshold.
func (h *HealthTracker) IsHealthy(channel string) bool {
    h.mu.RLock()
    defer h.mu.RUnlock()

    return h.health[channel] >= 0.5 && h.consecutive[channel] < failureThreshold
}

func (h *HealthTracker) recalc(channel string) {
    // Health = 1 - (failureRatio * 0.5) - (consecutive * 0.1)
    fail := h.totalFail[channel]
    success := h.totalSuccess[channel]
    total := fail + success
    if total == 0 {
        h.health[channel] = maxHealth
        return
    }

    ratio := float64(fail) / float64(total)
    consecutive := h.consecutive[channel]

    h.health[channel] = math.Max(minHealth,
        math.Min(maxHealth, 1.0 - (ratio*0.5) - (float64(consecutive)*0.1)))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/health_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/health.go internal/core/health_test.go
git commit -m "feat(core): add HealthTracker for channel health monitoring"
```

---

### Task 5: Core — Router (Weighted Random + Health-Aware)

**Files:**
- Create: `internal/core/router.go`
- Test: `internal/core/router_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/router_test.go
package core

import (
    "testing"
)

func TestWeightedRandomRouter(t *testing.T) {
    reg := NewPluginRegistry()
    health := NewHealthTracker()
    router := NewRouter(reg, health)

    // Register mock channels
    ch1 := &mockChannel{name: "ch1"}
    ch2 := &mockChannel{name: "ch2"}
    reg.Register(ch1)
    reg.Register(ch2)

    // ch1 healthy, ch2 healthy — both should be selectable
    selected := make(map[string]int)
    for i := 0; i < 1000; i++ {
        ch, err := router.Route()
        if err != nil {
            t.Fatalf("route error: %v", err)
        }
        selected[ch.Name()]++
    }

    // Both should be selected at least once
    if selected["ch1"] == 0 || selected["ch2"] == 0 {
        t.Errorf("both channels should be selected: ch1=%d ch2=%d", selected["ch1"], selected["ch2"])
    }

    // When one channel is unhealthy, only the healthy one should be selected
    for i := 0; i < 6; i++ {
        health.RecordFailure("ch1")
    }
    selected = make(map[string]int)
    for i := 0; i < 100; i++ {
        ch, err := router.Route()
        if err != nil {
            t.Fatalf("route error when ch1 unhealthy: %v", err)
        }
        selected[ch.Name()]++
    }
    if selected["ch1"] != 0 {
        t.Errorf("ch1 should not be selected when unhealthy: got %d", selected["ch1"])
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/router_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/core/router.go
package core

import (
    "errors"
    "math/rand"
    "sync"

    "goloop/internal/model"
)

// Router selects the best channel using weighted random with health awareness.
type Router struct {
    reg    *PluginRegistry
    health *HealthTracker
    mu     sync.RWMutex
}

// NewRouter creates a router with the given registry and health tracker.
func NewRouter(reg *PluginRegistry, health *HealthTracker) *Router {
    return &Router{reg: reg, health: health}
}

// Route selects a channel using weighted random.
// Channels with zero health (IsHealthy returns false) are excluded.
// Returns an error if no healthy channels are available.
func (r *Router) Route() (Channel, error) {
    channels := r.reg.List()

    // Filter to healthy channels only
    var candidates []Channel
    var weights []int
    var totalWeight int

    for _, ch := range channels {
        if !ch.IsAvailable() {
            continue
        }
        score := r.health.HealthScore(ch.Name())
        if score <= 0 {
            continue
        }
        // Weight = healthScore * channel's own weight factor (default 100)
        weight := int(score * 100)
        if weight <= 0 {
            weight = 1
        }
        candidates = append(candidates, ch)
        weights = append(weights, weight)
        totalWeight += weight
    }

    if len(candidates) == 0 {
        return nil, errors.New("router: no healthy channels available")
    }

    // Weighted random selection
    n := rand.Intn(totalWeight)
    var cumulative int
    for i, ch := range candidates {
        cumulative += weights[i]
        if n < cumulative {
            return ch, nil
        }
    }

    return candidates[len(candidates)-1], nil
}

// RouteForModel selects the best channel for a specific model.
// Falls back to general Route if no model-specific preference is configured.
func (r *Router) RouteForModel(modelName string) (Channel, error) {
    // Currently just uses Route. Model-specific routing can be added later.
    return r.Route()
}

// RecordResult updates health based on whether the call succeeded.
func (r *Router) RecordResult(channel string, success bool, latencyMs int64) {
    if success {
        r.health.RecordSuccess(channel)
    } else {
        r.health.RecordFailure(channel)
    }
    r.health.RecordLatency(channel, time.Duration(latencyMs*1e6))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/router_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/router.go internal/core/router_test.go
git commit -m "feat(core): add weighted random + health-aware Router"
```

---

### Task 6: Core — JWT Issuer and Validator

**Files:**
- Create: `internal/core/jwt.go`
- Test: `internal/core/jwt_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/jwt_test.go
package core

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestJWTLifecycle(t *testing.T) {
    issuer := NewJWTIssuer("test-secret-key", 1*time.Hour)

    // Issue a token
    claims := &JWTClaims{
        Subject:   "user-123",
        APIKey:    "user-api-key",
        Channel:   "kieai",
        ExpiresAt: time.Now().Add(1 * time.Hour),
    }
    token, err := issuer.Issue(claims)
    if err != nil {
        t.Fatalf("Issue error: %v", err)
    }
    if token == "" {
        t.Fatal("token should not be empty")
    }

    // Validate the token
    parsed, err := issuer.Validate(token)
    if err != nil {
        t.Fatalf("Validate error: %v", err)
    }
    if parsed.Subject != "user-123" {
        t.Errorf("subject mismatch: got %q", parsed.Subject)
    }
    if parsed.APIKey != "user-api-key" {
        t.Errorf("apiKey mismatch: got %q", parsed.APIKey)
    }

    // Invalid token
    _, err = issuer.Validate("invalid-token")
    if err == nil {
        t.Errorf("expected error for invalid token")
    }
}

func TestJWTMiddleware(t *testing.T) {
    issuer := NewJWTIssuer("secret", 1*time.Hour)
    token, _ := issuer.Issue(&JWTClaims{
        Subject:   "user-1",
        APIKey:    "key-1",
        ExpiresAt: time.Now().Add(1 * time.Hour),
    })

    var captured *JWTClaims
    nextCalled := false

    mw := JWT middleware{
        issuer: issuer,
        next: func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request) {
            captured = claims
            nextCalled = true
        },
    }

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mw.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKeyClaims, captured)))
    })

    // Valid token
    req := httptest.NewRequest("POST", "/", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    mw.ServeHTTP(nil, req)

    // Missing token
    req2 := httptest.NewRequest("POST", "/", nil)
    // no auth header — should write 401
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/jwt_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/core/jwt.go
package core

import (
    "context"
    "errors"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type contextKey string

const contextKeyClaims contextKey = "jwt_claims"

// JWTClaims represents the claims embedded in the JWT.
type JWTClaims struct {
    jwt.RegisteredClaims
    // APIKey is the per-user API key stored in the token
    APIKey string `json:"api_key,omitempty"`
    // Channel optionally restricts which channel this token can use
    Channel string `json:"channel,omitempty"`
    // Quota optionally embeds remaining quota
    Quota int64 `json:"quota,omitempty"`
}

// JWTIssuer creates and validates JWTs.
type JWTIssuer struct {
    secret []byte
    expiry time.Duration
}

// NewJWTIssuer creates a new issuer with the given secret and default token expiry.
func NewJWTIssuer(secret string, expiry time.Duration) *JWTIssuer {
    return &JWTIssuer{secret: []byte(secret), expiry: expiry}
}

// Issue creates a new JWT for the given claims.
func (j *JWTIssuer) Issue(claims *JWTClaims) (string, error) {
    if claims.Subject == "" {
        return "", errors.New("jwt: subject is required")
    }
    if claims.ExpiresAt.IsZero() {
        claims.ExpiresAt = time.Now().Add(j.expiry)
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(j.secret)
}

// Validate parses and validates a JWT. Returns the claims if valid.
func (j *JWTIssuer) Validate(tokenString string) (*JWTClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return j.secret, nil
    })
    if err != nil {
        return nil, fmt.Errorf("jwt: validate failed: %w", err)
    }
    claims, ok := token.Claims.(*JWTClaims)
    if !ok || !token.Valid {
        return nil, errors.New("jwt: invalid token")
    }
    return claims, nil
}

// ExtractBearerToken extracts the token from an Authorization: Bearer header.
func ExtractBearerToken(r *http.Request) string {
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return ""
}

// JWTMiddleware validates JWTs on incoming requests and injects claims into context.
type JWTMiddleware struct {
    issuer *JWTIssuer
    next   func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request)
}

// NewJWTMiddleware creates a middleware that validates JWTs and calls next with claims.
func NewJWTMiddleware(issuer *JWTIssuer, next func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request)) *JWTMiddleware {
    return &JWTMiddleware{issuer: issuer, next: next}
}

func (m *JWTMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    token := ExtractBearerToken(r)
    if token == "" {
        writeGoogleError(w, http.StatusUnauthorized, "JWT token required", "UNAUTHENTICATED")
        return
    }

    claims, err := m.issuer.Validate(token)
    if err != nil {
        writeGoogleError(w, http.StatusUnauthorized, err.Error(), "UNAUTHENTICATED")
        return
    }

    ctx := context.WithValue(r.Context(), contextKeyClaims, claims)
    m.next(ctx, claims, w, r.WithContext(ctx))
}

// GetClaims retrieves JWT claims from context. Returns nil if not present.
func GetClaims(ctx context.Context) *JWTClaims {
    if claims, ok := ctx.Value(contextKeyClaims).(*JWTClaims); ok {
        return claims
    }
    return nil
}

func writeGoogleError(w http.ResponseWriter, code int, message, status string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write([]byte(fmt.Sprintf(`{"error":{"code":%d,"message":%q,"status":%q}}`, code, message, status)))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/jwt_test.go -v`
Expected: PASS (after adding `github.com/golang-jwt/jwt/v5` dependency)

- [ ] **Step 5: Add JWT dependency and commit**

```bash
cd /Users/botycookie/ai/goloop && go get github.com/golang-jwt/jwt/v5
git add go.mod go.sum
git commit -m "feat(core): add JWT issuer, validator, and middleware"
```

---

## Chunk 2: KIE.AI Channel Plugin (with Multi-Account Pool)

### Task 7: KIE.AI — AccountPool with Weighted Random

**Files:**
- Create: `internal/channels/kieai/accountPool.go`
- Test: `internal/channels/kieai/accountPool_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/kieai/accountPool_test.go
package kieai

import (
    "testing"
)

func TestWeightedRandomAccountPool(t *testing.T) {
    pool := NewAccountPool()

    pool.AddAccount("key-1", 50)
    pool.AddAccount("key-2", 30)
    pool.AddAccount("key-3", 20)

    selected := make(map[string]int)
    for i := 0; i < 1000; i++ {
        acc, err := pool.Select()
        if err != nil {
            t.Fatalf("Select error: %v", err)
        }
        selected[acc.APIKey()]++
    }

    // key-1 should be most selected, key-3 least
    if selected["key-1"] <= selected["key-3"] {
        t.Errorf("key-1 should be selected more than key-3: key-1=%d key-3=%d", selected["key-1"], selected["key-3"])
    }
    if selected["key-2"] <= selected["key-3"] {
        t.Errorf("key-2 should be selected more than key-3: key-2=%d key-3=%d", selected["key-2"], selected["key-3"])
    }

    // Return accounts
    all := pool.List()
    for _, acc := range all {
        pool.Return(acc, true)
    }

    // Mark one account unhealthy and verify it's excluded
    unhealthy, _ := pool.Select()
    for i := 0; i < 5; i++ {
        pool.Return(unhealthy, false)
    }

    // After 5 failures, the unhealthy account should be excluded
    selectedUnhealthy := 0
    for i := 0; i < 100; i++ {
        acc, _ := pool.Select()
        if acc.APIKey() == unhealthy.APIKey() {
            selectedUnhealthy++
        }
    }
    if selectedUnhealthy > 0 {
        t.Errorf("unhealthy account should not be selected: got %d selections", selectedUnhealthy)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/accountPool_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/channels/kieai/accountPool.go
package kieai

import (
    "errors"
    "math/rand"
    "sync"
)

// kieAccount implements the core.Account interface for KIE.AI accounts.
type kieAccount struct {
    apiKey       string
    weight       int
    usageCount   int
    failCount    int
    healthy      bool
    mu           sync.RWMutex
}

func (a *kieAccount) APIKey() string    { return a.apiKey }
func (a *kieAccount) Weight() int       { return a.weight }
func (a *kieAccount) UsageCount() int   { return a.usageCount }
func (a *kieAccount) HealthScore() float64 {
    a.mu.RLock()
    defer a.mu.RUnlock()
    if a.failCount == 0 {
        return 1.0
    }
    return 1.0 - float64(a.failCount)*0.2
}
func (a *kieAccount) IsHealthy() bool {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.healthy && a.failCount < 5
}

func (a *kieAccount) IncUsage() {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.usageCount++
}
func (a *kieAccount) RecordFailure() {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.failCount++
    if a.failCount >= 5 {
        a.healthy = false
    }
}
func (a *kieAccount) RecordSuccess() {
    a.mu.Lock()
    defer a.mu.Unlock()
    a.failCount = max(0, a.failCount-1)
    a.healthy = true
}

// AccountPool manages multiple KIE.AI accounts with weighted random selection.
type AccountPool struct {
    mu       sync.RWMutex
    accounts []*kieAccount
}

// NewAccountPool creates an empty account pool.
func NewAccountPool() *AccountPool {
    return &AccountPool{}
}

// AddAccount adds a new account with the given API key and weight.
func (p *AccountPool) AddAccount(apiKey string, weight int) {
    p.mu.Lock()
    defer p.mu.Unlock()

    p.accounts = append(p.accounts, &kieAccount{
        apiKey:  apiKey,
        weight:  weight,
        healthy: true,
    })
}

// Select returns an account using weighted random, excluding unhealthy accounts.
func (p *AccountPool) Select() (Account, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()

    candidates := make([]Account, 0, len(p.accounts))
    weights := make([]int, 0, len(p.accounts))
    totalWeight := 0

    for _, acc := range p.accounts {
        if acc.IsHealthy() {
            weight := acc.Weight() * int(acc.HealthScore()*100) / 100
            if weight <= 0 {
                weight = 1
            }
            candidates = append(candidates, acc)
            weights = append(weights, weight)
            totalWeight += weight
        }
    }

    if len(candidates) == 0 {
        return nil, errors.New("kieai: no healthy accounts available")
    }

    n := rand.Intn(totalWeight)
    var cumulative int
    for i, acc := range candidates {
        cumulative += weights[i]
        if n < cumulative {
            return acc, nil
        }
    }
    return candidates[len(candidates)-1], nil
}

// Return returns the account to the pool after use with the result.
func (p *AccountPool) Return(acc Account, success bool) {
    if success {
        acc.RecordSuccess()
    } else {
        acc.RecordFailure()
    }
}

// List returns all accounts.
func (p *AccountPool) List() []Account {
    p.mu.RLock()
    defer p.mu.RUnlock()

    ret := make([]Account, len(p.accounts))
    for i, a := range p.accounts {
        ret[i] = a
    }
    return ret
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/accountPool_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/kieai/accountPool.go internal/channels/kieai/accountPool_test.go
git commit -m "feat(kieai): add weighted random AccountPool with health tracking"
```

---

### Task 8: KIE.AI — Channel Plugin

**Files:**
- Create: `internal/channels/kieai/channel.go`
- Test: `internal/channels/kieai/channel_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/kieai/channel_test.go
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

func TestKIEAIChannel(t *testing.T) {
    // Mock KIE.AI server
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]any{
            "code": 200,
            "data": map[string]string{"taskId": "task-abc"},
        })
    }))
    defer srv.Close()

    pool := NewAccountPool()
    pool.AddAccount("test-key", 100)

    ch := NewChannel(srv.URL, pool, Config{
        InitialInterval: 10 * time.Millisecond,
        MaxInterval:     50 * time.Millisecond,
        MaxWaitTime:     5 * time.Second,
    })

    if ch.Name() != "kieai" {
        t.Errorf("expected name kieai, got %q", ch.Name())
    }

    if !ch.IsAvailable() {
        t.Errorf("channel should be available")
    }

    req := &model.GoogleRequest{
        Contents: []model.Content{
            {Parts: []model.Part{{Text: "test"}}},
        },
    }

    // Test SubmitTask
    taskID, err := ch.SubmitTask(context.Background(), "test-key", req, "gemini-3.1-flash-image-preview")
    if err != nil {
        t.Fatalf("SubmitTask error: %v", err)
    }
    if taskID != "task-abc" {
        t.Errorf("taskID: got %q", taskID)
    }

    _ = ch.HealthScore() // should not panic
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/channel_test.go -v`
Expected: FAIL (channel.go doesn't exist yet)

- [ ] **Step 3: Write KIE.AI channel implementation**

```go
// internal/channels/kieai/channel.go
package kieai

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log/slog"
    "math/rand"
    "net/http"
    "sync"
    "time"

    "goloop/internal/model"
    "goloop/internal/storage"
)

// Config holds KIE.AI channel configuration.
type Config struct {
    BaseURL          string
    Timeout          time.Duration
    InitialInterval  time.Duration
    MaxInterval      time.Duration
    MaxWaitTime      time.Duration
    RetryAttempts    int
}

// Channel implements the core.Channel interface for KIE.AI.
type Channel struct {
    name        string
    baseURL     string
    httpClient  *http.Client
    pool        *AccountPool
    storage     *storage.Store
    reqTransform *RequestTransformer
    respTransform *ResponseTransformer
    cfg         Config
    mu          sync.RWMutex
    healthy     bool
}

// Account implements core.Account for the kieai package.
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

// NewChannel creates a new KIE.AI channel plugin.
func NewChannel(baseURL string, pool *AccountPool, cfg Config) *Channel {
    if cfg.InitialInterval == 0 {
        cfg.InitialInterval = 2 * time.Second
    }
    if cfg.MaxInterval == 0 {
        cfg.MaxInterval = 10 * time.Second
    }
    if cfg.MaxWaitTime == 0 {
        cfg.MaxWaitTime = 120 * time.Second
    }
    if cfg.RetryAttempts == 0 {
        cfg.RetryAttempts = 3
    }

    ch := &Channel{
        name:     "kieai",
        baseURL:  baseURL,
        httpClient: &http.Client{
            Timeout: cfg.Timeout,
            Transport: &http.Transport{
                MaxIdleConns:    100,
                IdleConnTimeout: 90 * time.Second,
            },
        },
        pool:    pool,
        cfg:     cfg,
        healthy: true,
    }

    // Build transformers
    modelMapping := map[string]ModelDefaults{
        "gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
        "gemini-3-pro-image-preview":     {KieAIModel: "nano-banana-pro", AspectRatio: "1:1", Resolution: "2K", OutputFormat: "png"},
        "gemini-2.5-flash-image":         {KieAIModel: "google/nano-banana", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
    }
    ch.reqTransform = NewRequestTransformer(modelMapping)
    ch.respTransform = NewResponseTransformer()

    return ch
}

func (ch *Channel) Name() string     { return ch.name }
func (ch *Channel) IsAvailable() bool {
    ch.mu.RLock()
    defer ch.mu.RUnlock()
    return ch.healthy
}

func (ch *Channel) HealthScore() float64 {
    ch.mu.RLock()
    defer ch.mu.RUnlock()
    if !ch.healthy {
        return 0
    }
    // Aggregate health from account pool
    accounts := ch.pool.List()
    if len(accounts) == 0 {
        return 0
    }
    var total float64
    for _, acc := range accounts {
        total += acc.HealthScore()
    }
    return total / float64(len(accounts))
}

// Generate makes a synchronous call — not supported for async KIE.AI.
func (ch *Channel) Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, modelName string) (*model.GoogleResponse, error) {
    return nil, errors.New("kieai: Generate not supported, use SubmitTask + PollTask")
}

// SubmitTask submits an image generation task.
func (ch *Channel) SubmitTask(ctx context.Context, apiKey string, req *model.GoogleRequest, modelName string) (string, error) {
    kieReq, err := ch.reqTransform.Transform(ctx, req, modelName)
    if err != nil {
        return "", fmt.Errorf("kieai: transform: %w", err)
    }

    body, err := json.Marshal(kieReq)
    if err != nil {
        return "", fmt.Errorf("kieai: marshal: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
        ch.baseURL+"/api/v1/jobs/createTask", bytes.NewReader(body))
    if err != nil {
        return "", fmt.Errorf("kieai: build request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+apiKey)

    resp, err := ch.httpClient.Do(httpReq)
    if err != nil {
        return "", fmt.Errorf("kieai: request: %w", err)
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
    if err != nil {
        return "", fmt.Errorf("kieai: read response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("kieai: HTTP %d: %s", resp.StatusCode, string(data))
    }

    var result struct {
        Code int    `json:"code"`
        Msg  string `json:"msg"`
        Data struct {
            TaskID string `json:"taskId"`
        } `json:"data"`
    }
    if err := json.Unmarshal(data, &result); err != nil {
        return "", fmt.Errorf("kieai: unmarshal: %w", err)
    }
    if result.Data.TaskID == "" {
        return "", fmt.Errorf("kieai: empty taskId: %s", result.Msg)
    }

    return result.Data.TaskID, nil
}

// PollTask polls until the task completes and returns the Google-formatted response.
func (ch *Channel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
    deadline := time.Now().Add(ch.cfg.MaxWaitTime)
    interval := ch.cfg.InitialInterval
    consecutiveFails := 0

    for {
        if time.Now().After(deadline) {
            return nil, fmt.Errorf("kieai: task %q timed out after %v", taskID, ch.cfg.MaxWaitTime)
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(interval):
        }

        record, err := ch.getTaskStatus(ctx, apiKey, taskID)
        if err != nil {
            consecutiveFails++
            slog.Warn("kieai: poll failed", "taskId", taskID, "fails", consecutiveFails, "err", err)
            if consecutiveFails >= ch.cfg.RetryAttempts {
                return nil, fmt.Errorf("kieai: task %q: %d consecutive failures: %w", taskID, consecutiveFails, err)
            }
            interval = min(interval*2, ch.cfg.MaxInterval)
            continue
        }
        consecutiveFails = 0

        switch record.State {
        case "success":
            if record.ResultJSON() == nil || len(record.ResultJSON().ResultURLs) == 0 {
                return nil, fmt.Errorf("kieai: task succeeded but no result URLs")
            }
            return ch.respTransform.ToGoogleResponse(ctx, record.ResultJSON().ResultURLs)

        case "fail":
            reason := record.FailReason
            if reason == "" {
                reason = "unknown failure"
            }
            return nil, fmt.Errorf("kieai: task %q failed: %s", taskID, reason)

        case "waiting", "queuing", "generating":
            interval = min(interval*2, ch.cfg.MaxInterval)
        }
    }
}

func (ch *Channel) getTaskStatus(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
    url := ch.baseURL + "/api/v1/jobs/recordInfo?taskId=" + taskID

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, err
    }
    httpReq.Header.Set("Authorization", "Bearer "+apiKey)

    resp, err := ch.httpClient.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
    if err != nil {
        return nil, err
    }
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("kieai: HTTP %d: %s", resp.StatusCode, string(data))
    }

    var result struct {
        Code int `json:"code"`
        Data struct {
            TaskID        string `json:"taskId"`
            State         string `json:"state"`
            ResultJSONRaw string `json:"resultJson,omitempty"`
            FailReason    string `json:"failReason,omitempty"`
        } `json:"data"`
    }
    if err := json.Unmarshal(data, &result); err != nil {
        return nil, err
    }

    return &model.KieAIRecordData{
        TaskID:        result.Data.TaskID,
        State:         result.Data.State,
        ResultJSONRaw: result.Data.ResultJSONRaw,
        FailReason:    result.Data.FailReason,
    }, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/channel_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/kieai/channel.go internal/channels/kieai/channel_test.go
git commit -m "feat(kieai): implement core.Channel plugin for KIE.AI with account pool"
```

---

### Task 9: KIE.AI — Request and Response Transformers

**Files:**
- Create: `internal/channels/kieai/requestTransformer.go`
- Create: `internal/channels/kieai/responseTransformer.go`
- Test: `internal/channels/kieai/requestTransformer_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/kieai/requestTransformer_test.go
package kieai

import (
    "context"
    "testing"

    "goloop/internal/model"
)

func TestKIEAIRequestTransform(t *testing.T) {
    mapping := map[string]ModelDefaults{
        "gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
    }
    rt := NewRequestTransformer(mapping)

    req := &model.GoogleRequest{
        Contents: []model.Content{
            {Parts: []model.Part{{Text: "draw a cat"}}},
        },
    }

    kieReq, err := rt.Transform(context.Background(), req, "gemini-3.1-flash-image-preview")
    if err != nil {
        t.Fatalf("Transform error: %v", err)
    }
    if kieReq.Model != "nano-banana-2" {
        t.Errorf("model mismatch: got %q", kieReq.Model)
    }
    if kieReq.Input.Prompt != "draw a cat" {
        t.Errorf("prompt mismatch: got %q", kieReq.Input.Prompt)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/requestTransformer_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write transformers**

```go
// internal/channels/kieai/requestTransformer.go
package kieai

import (
    "context"
    "encoding/base64"
    "fmt"
    "strings"

    "goloop/internal/model"
)

// ModelDefaults holds KIE.AI model configuration.
type ModelDefaults struct {
    KieAIModel   string
    AspectRatio  string
    Resolution   string
    OutputFormat string
}

// RequestTransformer converts Google requests to KIE.AI format.
type RequestTransformer struct {
    modelMapping map[string]ModelDefaults
}

func NewRequestTransformer(mapping map[string]ModelDefaults) *RequestTransformer {
    return &RequestTransformer{modelMapping: mapping}
}

func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, googleModel string) (*model.KieAICreateTaskRequest, error) {
    defaults, ok := t.modelMapping[googleModel]
    if !ok {
        return nil, fmt.Errorf("kieai: unknown model %q", googleModel)
    }

    var textParts []string
    for _, content := range req.Contents {
        for _, part := range content.Parts {
            if part.Text != "" {
                textParts = append(textParts, part.Text)
            }
        }
    }

    input := model.KieAIInput{
        Prompt:       strings.Join(textParts, " "),
        AspectRatio:  defaults.AspectRatio,
        Resolution:   defaults.Resolution,
        OutputFormat: defaults.OutputFormat,
    }

    if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
        ic := req.GenerationConfig.ImageConfig
        if ic.AspectRatio != "" {
            input.AspectRatio = ic.AspectRatio
        }
        if ic.ImageSize != "" {
            input.Resolution = ic.ImageSize
        }
        if ic.OutputFormat != "" {
            input.OutputFormat = ic.OutputFormat
        }
    }

    return &model.KieAICreateTaskRequest{
        Model: defaults.KieAIModel,
        Input: input,
    }, nil
}

// DecodeBase64 decodes a base64 string, trying standard then URL-safe encoding.
func DecodeBase64(data string) ([]byte, error) {
    raw, err := base64.StdEncoding.DecodeString(data)
    if err != nil {
        raw, err = base64.URLEncoding.DecodeString(data)
    }
    return raw, err
}
```

```go
// internal/channels/kieai/responseTransformer.go
package kieai

import (
    "context"
    "encoding/base64"
    "fmt"
    "sync"

    "goloop/internal/model"
    "goloop/internal/storage"
)

// ResponseTransformer converts KIE.AI results to Google format.
type ResponseTransformer struct {
    store *storage.Store
}

func NewResponseTransformer() *ResponseTransformer {
    return &ResponseTransformer{}
}

func (t *ResponseTransformer) ToGoogleResponse(ctx context.Context, resultURLs []string) (*model.GoogleResponse, error) {
    if len(resultURLs) == 0 {
        return nil, fmt.Errorf("no result URLs")
    }

    type imgResult struct {
        idx  int
        data []byte
        err  error
    }

    results := make([]imgResult, len(resultURLs))
    var wg sync.WaitGroup
    ch := make(chan imgResult, len(resultURLs))

    for i, url := range resultURLs {
        wg.Add(1)
        go func(idx int, u string) {
            defer wg.Done()
            if t.store != nil {
                data, err := t.store.DownloadToBytes(ctx, u)
                ch <- imgResult{idx: idx, data: data, err: err}
            } else {
                ch <- imgResult{idx: idx, data: []byte{}, err: nil}
            }
        }(i, url)
    }

    go func() {
        wg.Wait()
        close(ch)
    }()

    for r := range ch {
        results[r.idx] = r
    }

    parts := []model.Part{
        {Text: fmt.Sprintf("Generated %d image(s) successfully.", len(resultURLs))},
    }
    for _, r := range results {
        if r.err != nil {
            return nil, fmt.Errorf("download image %d: %w", r.idx, r.err)
        }
        encoded := base64.StdEncoding.EncodeToString(r.data)
        parts = append(parts, model.Part{
            InlineData: &model.InlineData{MimeType: "image/png", Data: encoded},
        })
    }

    return &model.GoogleResponse{
        Candidates: []model.Candidate{
            {Content: model.Content{Parts: parts}, FinishReason: "STOP"},
        },
    }, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/requestTransformer_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/kieai/requestTransformer.go internal/channels/kieai/responseTransformer.go
git commit -m "feat(kieai): add request and response transformers for KIE.AI channel"
```

---

## Chunk 3: Handler Refactor and Server Wiring

### Task 10: Handler — Refactor to Use Core

**Files:**
- Modify: `internal/handler/gemini_handler.go`
- Modify: `internal/handler/gemini_handler_test.go`

- [ ] **Step 1: Write the failing test**

```go
// The existing handler tests will serve as regression tests.
// After refactoring, run: go test ./internal/handler/... -v
// All existing tests must continue to pass.
```

- [ ] **Step 2: Write new handler that delegates to core**

```go
// internal/handler/gemini_handler.go
// Refactored to use core.Router and core.JWTMiddleware.
// Key changes:
// - Remove direct kieai client/poller references
// - Delegate to core.Router.Route() + channel.Generate/PollTask
// - JWT validation via core.JWTMiddleware
// - Preserve existing API surface (same HTTP endpoints)
```

```go
// internal/handler/gemini_handler.go
package handler

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io"
    "log/slog"
    "net/http"
    "strings"

    "goloop/internal/core"
    "goloop/internal/model"
)

const maxRequestBodyBytes = 10 * 1024 * 1024 // 10MB

// GeminiHandler is the main HTTP handler, now backed by core routing.
type GeminiHandler struct {
    router   *core.Router
    registry *core.PluginRegistry
    issuer   *core.JWTIssuer
    storage  *storage.Store
}

// NewGeminiHandler creates a handler wired to the core.
func NewGeminiHandler(router *core.Router, registry *core.PluginRegistry, issuer *core.JWTIssuer, storage *storage.Store) *GeminiHandler {
    return &GeminiHandler{
        router:   router,
        registry: registry,
        issuer:   issuer,
        storage:  storage,
    }
}

func (h *GeminiHandler) RegisterRoutes(mux *http.ServeMux) {
    // JWT-protected routes
    protected := core.NewJWTMiddleware(h.issuer, h.handleProtected)
    mux.Handle("POST /v1beta/models/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        protected.ServeHTTP(w, r)
    }))

    // Public routes
    mux.HandleFunc("GET /v1beta/models", h.handleListModels)
    mux.HandleFunc("GET /health", h.handleHealth)

    // Admin: issue token (for testing / internal use)
    mux.HandleFunc("POST /admin/issue-token", h.handleIssueToken)
}

func (h *GeminiHandler) handleProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
    googleModel := extractModel(r.URL.Path)
    if googleModel == "" {
        writeGoogleError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
        return
    }

    requestID := generateRequestID()
    log := slog.With("requestId", requestID, "googleModel", googleModel)

    // Parse request body
    bodyBytes, err := readBody(r)
    if err != nil {
        writeGoogleError(w, http.StatusBadRequest, err.Error(), "INVALID_ARGUMENT")
        return
    }

    var googleReq model.GoogleRequest
    if err := json.Unmarshal(bodyBytes, &googleReq); err != nil {
        writeGoogleError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "INVALID_ARGUMENT")
        return
    }

    // Route to channel
    ch, err := h.router.RouteForModel(googleModel)
    if err != nil {
        log.Error("no channel available", "err", err)
        writeGoogleError(w, http.StatusServiceUnavailable, "no channel available", "INTERNAL")
        return
    }

    // Use JWT's embedded API key if available, else fall back to claim
    apiKey := claims.APIKey
    if apiKey == "" {
        apiKey = extractAPIKey(r) // fallback to original header-based key
    }

    start := time.Now()

    // Submit task
    taskID, err := ch.SubmitTask(ctx, apiKey, &googleReq, googleModel)
    if err != nil {
        log.Error("submit failed", "channel", ch.Name(), "err", err)
        h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
        writeGoogleError(w, http.StatusInternalServerError, err.Error(), "INTERNAL")
        return
    }

    // Poll for result
    resp, err := ch.PollTask(ctx, apiKey, taskID)
    if err != nil {
        log.Error("poll failed", "channel", ch.Name(), "err", err)
        h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
        writeGoogleError(w, http.StatusInternalServerError, err.Error(), "INTERNAL")
        return
    }

    h.router.RecordResult(ch.Name(), true, time.Since(start).Milliseconds())
    log.Info("request completed", "channel", ch.Name(), "latency", time.Since(start))

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
}

func (h *GeminiHandler) handleIssueToken(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Subject string `json:"subject"`
        APIKey  string `json:"api_key"`
        Channel string `json:"channel"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeGoogleError(w, http.StatusBadRequest, err.Error(), "INVALID_ARGUMENT")
        return
    }

    claims := &core.JWTClaims{
        Subject: req.Subject,
        APIKey:  req.APIKey,
        Channel: req.Channel,
    }
    token, err := h.issuer.Issue(claims)
    if err != nil {
        writeGoogleError(w, http.StatusInternalServerError, err.Error(), "INTERNAL")
        return
    }

    json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *GeminiHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
    models := []string{
        "gemini-3.1-flash-image-preview",
        "gemini-3-pro-image-preview",
        "gemini-2.5-flash-image",
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]any{"models": models})
}

func (h *GeminiHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"status":"ok"}`))
}

func extractModel(path string) string {
    suffix := strings.TrimPrefix(path, "/v1beta/models/")
    model, _, found := strings.Cut(suffix, ":")
    if !found {
        return ""
    }
    return model
}

func readBody(r *http.Request) ([]byte, error) {
    limited := io.LimitReader(r.Body, maxRequestBodyBytes+1)
    data, err := io.ReadAll(limited)
    if err != nil {
        return nil, err
    }
    if len(data) > maxRequestBodyBytes {
        return nil, errors.New("request body too large")
    }
    return data, nil
}

func extractAPIKey(r *http.Request) string {
    if key := r.Header.Get("x-goog-api-key"); key != "" {
        return key
    }
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return ""
}

func writeGoogleError(w http.ResponseWriter, code int, message, status string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(model.GoogleError{
        Error: model.GoogleErrorDetail{Code: code, Message: message, Status: status},
    })
}

func generateRequestID() string {
    b := make([]byte, 8)
    rand.Read(b)
    return hex.EncodeToString(b)
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/botycookie/ai/goloop && go build ./... && go test ./internal/handler/... -v`

- [ ] **Step 4: Commit**

```bash
git add internal/handler/gemini_handler.go internal/handler/gemini_handler_test.go
git commit -m "refactor(handler): delegate to core.Router and core.JWTMiddleware"
```

---

### Task 11: Config — Multi-Channel and Multi-Account Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config/config.yaml`

- [ ] **Step 1: Write updated config structures**

```go
// internal/config/config.go
// Add to Config struct:

type Config struct {
    Server       ServerConfig
    JWT          JWTConfig
    Channels     map[string]ChannelConfig  // e.g. "kieai" -> KIE.AI config
    ModelMapping map[string]ModelDefaults
}

type JWTConfig struct {
    Secret string
    Expiry time.Duration
}

type ChannelConfig struct {
    Type      string                  // e.g. "kieai", "gemini-direct"
    BaseURL   string
    Timeout   time.Duration
    Accounts  []AccountConfig         // per-channel account list
    Poller    PollerConfig
    Weight    int                     // channel weight for routing (default 100)
}

type AccountConfig struct {
    APIKey string
    Weight int  // selection weight (default 100)
}
```

- [ ] **Step 2: Write YAML config example**

```yaml
# config/config.yaml
server:
  port: 8080
  read_timeout: 130s
  write_timeout: 130s

jwt:
  secret: "${JWT_SECRET}"
  expiry: 24h

channels:
  kieai:
    type: kieai
    base_url: https://api.kie.ai
    timeout: 120s
    weight: 100
    poller:
      initial_interval: 2s
      max_interval: 10s
      max_wait_time: 120s
      retry_attempts: 3
    accounts:
      - api_key: "${KIEAI_KEY_1}"
        weight: 50
      - api_key: "${KIEAI_KEY_2}"
        weight: 30
      - api_key: "${KIEAI_KEY_3}"
        weight: 20

  # Future channel:
  # gemini-direct:
  #   type: gemini-direct
  #   base_url: https://generativelanguage.googleapis.com
  #   weight: 50
  #   accounts:
  #     - api_key: "${GEMINI_KEY}"
  #       weight: 100

model_mapping:
  gemini-3.1-flash-image-preview:
    channel: kieai
    kieai_model: nano-banana-2
    aspect_ratio: "1:1"
    resolution: "1K"
    output_format: png
```

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go config/config.yaml
git commit -m "feat(config): add multi-channel and multi-account configuration"
```

---

### Task 12: Server — Wire Everything Together

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Write the wired main.go**

```go
// cmd/server/main.go
package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "goloop/internal/channels/kieai"
    "goloop/internal/config"
    "goloop/internal/core"
    "goloop/internal/handler"
    "goloop/internal/storage"
)

func main() {
    configPath := flag.String("config", "config/config.yaml", "path to config file")
    flag.Parse()

    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

    cfg, err := config.Load(*configPath)
    if err != nil {
        slog.Error("failed to load config", "err", err)
        os.Exit(1)
    }

    // Build core infrastructure
    registry := core.NewPluginRegistry()
    health := core.NewHealthTracker()
    router := core.NewRouter(registry, health)
    issuer := core.NewJWTIssuer(cfg.JWT.Secret, cfg.JWT.Expiry)

    // Build storage
    store, err := storage.NewStore(cfg.Storage.LocalPath, cfg.Storage.BaseURL)
    if err != nil {
        slog.Error("failed to init storage", "err", err)
        os.Exit(1)
    }

    // Bootstrap channels from config
    for name, chCfg := range cfg.Channels {
        switch chCfg.Type {
        case "kieai":
            pool := kieai.NewAccountPool()
            for _, accCfg := range chCfg.Accounts {
                pool.AddAccount(accCfg.APIKey, accCfg.Weight)
            }
            kieCh := kieai.NewChannel(chCfg.BaseURL, pool, kieai.Config{
                BaseURL:         chCfg.BaseURL,
                Timeout:         chCfg.Timeout,
                InitialInterval: chCfg.Poller.InitialInterval,
                MaxInterval:     chCfg.Poller.MaxInterval,
                MaxWaitTime:     chCfg.Poller.MaxWaitTime,
                RetryAttempts:   chCfg.Poller.RetryAttempts,
            })
            registry.Register(kieCh)
            slog.Info("channel registered", "name", name, "accounts", len(chCfg.Accounts))

        default:
            slog.Warn("unknown channel type, skipping", "name", name, "type", chCfg.Type)
        }
    }

    if len(registry.List()) == 0 {
        slog.Error("no channels registered")
        os.Exit(1)
    }

    // Build handler
    geminiHandler := handler.NewGeminiHandler(router, registry, issuer, store)
    mux := http.NewServeMux()
    geminiHandler.RegisterRoutes(mux)
    mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(cfg.Storage.LocalPath))))

    server := &http.Server{
        Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
        Handler:      mux,
        ReadTimeout:  cfg.Server.ReadTimeout,
        WriteTimeout: cfg.Server.WriteTimeout,
    }

    go func() {
        slog.Info("server starting", "port", cfg.Server.Port)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("server error", "err", err)
            os.Exit(1)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    slog.Info("shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    server.Shutdown(ctx)
    slog.Info("server stopped")
}
```

- [ ] **Step 2: Build and test**

```bash
cd /Users/botycookie/ai/goloop && go build ./... && go test ./... -timeout 60s
```

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(server): wire core plugin system, multi-channel router, and JWT issuer"
```

---

## Implementation Order

```
Chunk 1 — Core (no external deps, purely interface-driven):
  Task 1 (Channel interface)
  Task 2 (Account interface)
  Task 3 (PluginRegistry)
  Task 4 (HealthTracker)
  Task 5 (Router)
  Task 6 (JWT — needs go get jwt/v5)

Chunk 2 — KIE.AI Plugin:
  Task 7 (AccountPool weighted random)
  Task 8 (KIE.AI Channel)
  Task 9 (Transformers)

Chunk 3 — Wiring:
  Task 10 (Handler refactor)
  Task 11 (Config update)
  Task 12 (main.go wire)
```

## Key Design Decisions

1. **Plugin boundary**: Core knows only the `Channel` interface. It never imports `kieai` or any other provider package.
2. **Account pool is per-channel**: Each channel plugin owns its account pool. The core only sees `Channel.HealthScore()`.
3. **JWT as pass-through**: The `JWTClaims.APIKey` field carries the per-user credential selected at token issuance time. The handler uses this API key when calling `channel.SubmitTask`.
4. **Health tracking**: Both channel-level and account-level health exist. Channel-level aggregates account health; account-level drives `Select()` exclusion.
5. **No changes to API surface**: The HTTP endpoints (`POST /v1beta/models/{model}:generateContent`, `GET /health`) remain identical. Only the internal implementation changes.
6. **Graceful degradation**: If the KIE.AI channel has no healthy accounts, `router.Route()` returns an error and the handler returns 503. Future channels can be added without changing handler code.
