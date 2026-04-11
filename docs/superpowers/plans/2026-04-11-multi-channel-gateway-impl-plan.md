# Multi-Channel Gateway — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a microkernel-style gateway that self-issues JWTs for auth, routes requests across multiple AI provider channels (starting with KIE.AI), rotates across multiple accounts per channel with weighted random selection, guarantees Gemini-compatible I/O, and includes health recovery with auto-probe and Admin UI.

**Architecture:** Core (interfaces + orchestration) is isolated from provider plugins. Each AI provider is a plugin implementing `Channel`. Each channel owns an `AccountPool`. Health recovery runs via a background `ChannelHealthReaper`. Admin UI is a separate module served alongside the API.

**Tech Stack:** Go 1.23, `github.com/golang-jwt/jwt/v5`, `gopkg.in/yaml.v3`, `net/http`, `log/slog`

---

## File Map

```
CREATE:
  internal/core/channel.go           — Channel interface
  internal/core/account.go         — Account + AccountPool interfaces
  internal/core/health.go          — HealthTracker
  internal/core/healthReaper.go   — Background probe goroutine
  internal/core/router.go         — Weighted random + health-aware router
  internal/core/jwt.go            — JWTIssuer, JWTMiddleware
  internal/core/pluginRegistry.go — PluginRegistry
  internal/channels/kieai/channel.go        — KIE.AI Channel plugin
  internal/channels/kieai/accountPool.go    — Per-channel AccountPool + account impl
  internal/channels/kieai/requestTransformer.go
  internal/channels/kieai/responseTransformer.go
  internal/handler/admin_handler.go — Admin API endpoints
  internal/admin/ui/index.html     — Admin SPA (HTML+CSS+JS, no build step)
  internal/admin/ui/styles.css
  internal/admin/ui/app.js
  cmd/server/main.go              — Rewired server entrypoint

MODIFY:
  go.mod                          — Add github.com/golang-jwt/jwt/v5
  internal/handler/gemini_handler.go — Delegate to core.Router, strip kieai direct refs
  internal/config/config.go       — Add multi-channel + multi-account config structs
  config/config.yaml              — Full multi-channel config
  internal/model/google.go        — Add StreamingResponse if not already present
  internal/model/kieai.go        — Add ResultJSON() method if not present
```

---

## Chunk 1: Core Interfaces

### Task 1: `core.Channel` Interface

**Files:** Create `internal/core/channel.go`, Test `internal/core/channel_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
    "context"
    "testing"

    "goloop/internal/model"
)

func TestChannelInterface_SatisfiedByMock(t *testing.T) {
    var _ Channel = &mockChannel{name: "test"}
    ch := &mockChannel{name: "kieai"}

    if ch.Name() != "kieai" { t.Errorf("Name mismatch") }
    if ch.HealthScore() != 1.0 { t.Errorf("HealthScore should be 1.0") }
    if !ch.IsAvailable() { t.Errorf("IsAvailable should be true") }

    resp, err := ch.Generate(context.Background(), "key", &model.GoogleRequest{}, "model")
    if err != nil { t.Fatalf("Generate returned error: %v", err) }
    _ = resp

    id, err := ch.SubmitTask(context.Background(), "key", &model.GoogleRequest{}, "model")
    if err != nil { t.Fatalf("SubmitTask returned error: %v", err) }
    if id != "task-mock" { t.Errorf("SubmitTask taskID mismatch") }

    _, err = ch.PollTask(context.Background(), "key", "task-1")
    if err != nil { t.Fatalf("PollTask returned error: %v", err) }
}

type mockChannel struct{ name string }

func (m *mockChannel) Name() string     { return m.name }
func (m *mockChannel) HealthScore() float64 { return 1.0 }
func (m *mockChannel) IsAvailable() bool  { return true }

func (m *mockChannel) Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (*model.GoogleResponse, error) {
    return &model.GoogleResponse{
        Candidates: []model.Candidate{
            {Content: model.Content{Parts: []model.Part{{Text: "mock"}}}, FinishReason: "STOP"},
        },
    }, nil
}

func (m *mockChannel) SubmitTask(ctx context.Context, apiKey string, req *model.GoogleRequest, model string) (string, error) {
    return "task-mock", nil
}

func (m *mockChannel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
    return &model.GoogleResponse{}, nil
}

func (m *mockChannel) Probe(account Account) bool {
    return true
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/channel_test.go -v`
Expected: FAIL — undefined: mockChannel / Channel

- [ ] **Step 3: Write the interface**

```go
package core

import (
    "context"

    "goloop/internal/model"
)

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
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/channel_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/channel.go internal/core/channel_test.go
git commit -m "feat(core): add Channel plugin interface with Probe method"
```

---

### Task 2: `core.Account` + `core.AccountPool` Interfaces

**Files:** Create `internal/core/account.go`, Test `internal/core/account_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestAccountInterface(t *testing.T) {
    var _ Account = &mockAccount{key: "k1", weight: 50}

    acc := &mockAccount{key: "k1", weight: 50}
    if acc.APIKey() != "k1" { t.Errorf("APIKey mismatch") }
    if acc.Weight() != 50 { t.Errorf("Weight mismatch") }
    if acc.UsageCount() != 0 { t.Errorf("UsageCount should start at 0") }
    if acc.HealthScore() != 1.0 { t.Errorf("HealthScore initial should be 1.0") }
    if !acc.IsHealthy() { t.Errorf("IsHealthy initial should be true") }

    acc.IncUsage()
    if acc.UsageCount() != 1 { t.Errorf("UsageCount should be 1 after IncUsage") }

    acc.RecordFailure()
    if acc.HealthScore() >= 1.0 { t.Errorf("HealthScore should decrease after failure") }
    if acc.consecutiveFailures != 1 { t.Errorf("consecutiveFailures should be 1") }

    acc.RecordSuccess()
    if acc.consecutiveFailures != 0 { t.Errorf("consecutiveFailures should reset after success") }
}

type mockAccount struct {
    key               string
    weight            int
    usageCount        int
    consecutiveFailures int
}

func (m *mockAccount) APIKey() string       { return m.key }
func (m *mockAccount) Weight() int          { return m.weight }
func (m *mockAccount) UsageCount() int     { return m.usageCount }
func (m *mockAccount) HealthScore() float64 {
    if m.consecutiveFailures == 0 { return 1.0 }
    return 1.0 - float64(m.consecutiveFailures)*0.2
}
func (m *mockAccount) IsHealthy() bool { return m.consecutiveFailures < 5 }
func (m *mockAccount) IncUsage()         { m.usageCount++ }
func (m *mockAccount) RecordFailure()     { m.consecutiveFailures++ }
func (m *mockAccount) RecordSuccess()     { m.consecutiveFailures = 0 }
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/account_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write the interfaces**

```go
package core

// Account represents a single API credential within a channel.
type Account interface {
    APIKey() string
    Weight() int
    UsageCount() int
    HealthScore() float64 // 0.0-1.0
    IsHealthy() bool

    IncUsage()
    RecordFailure()  // increments consecutive failure counter
    RecordSuccess()   // resets consecutive failure counter
}

// AccountPool selects accounts using weighted random with health awareness.
type AccountPool interface {
    Select() (Account, error)    // weighted random, excludes unhealthy
    Return(account Account, success bool)
    List() []Account            // all accounts including unhealthy
}
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/account_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/account.go internal/core/account_test.go
git commit -m "feat(core): add Account and AccountPool interfaces"
```

---

### Task 3: `core.HealthTracker`

**Files:** Create `internal/core/health.go`, Test `internal/core/health_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
    "testing"
    "time"
)

func TestHealthTracker(t *testing.T) {
    ht := NewHealthTracker()

    if !ht.IsHealthy("ch1") { t.Errorf("ch1 should be healthy initially") }

    ht.RecordFailure("ch1")
    ht.RecordFailure("ch1")
    if ht.HealthScore("ch1") >= 0.7 { t.Errorf("health should drop after failures") }

    ht.RecordSuccess("ch1")
    if ht.HealthScore("ch1") < 0.5 { t.Errorf("health should improve after success") }

    // 5 consecutive failures → unhealthy
    for i := 0; i < 6; i++ {
        ht.RecordFailure("ch2")
    }
    if ht.IsHealthy("ch2") { t.Errorf("ch2 should be unhealthy after 6 failures") }

    // Latency tracking
    ht.RecordLatency("ch1", 200*time.Millisecond)
    ht.RecordLatency("ch1", 400*time.Millisecond)
    avg := ht.AverageLatency("ch1")
    if avg < 250*time.Millisecond || avg > 350*time.Millisecond {
        t.Errorf("average latency out of range: got %v", avg)
    }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/health_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write HealthTracker**

```go
package core

import (
    "math"
    "sync"
    "time"
)

const (
    failureDecay       = 0.2
    failureThreshold    = 5
    maxHealth           = 1.0
    minHealth           = 0.0
)

// HealthTracker records per-channel success/failure/latency.
type HealthTracker struct {
    mu            sync.RWMutex
    consecutive   map[string]int
    totalFail     map[string]int
    totalSuccess  map[string]int
    latencies     map[string][]time.Duration
    health        map[string]float64
}

func NewHealthTracker() *HealthTracker {
    return &HealthTracker{
        consecutive:  make(map[string]int),
        totalFail:    make(map[string]int),
        totalSuccess: make(map[string]int),
        latencies:    make(map[string][]time.Duration),
        health:       make(map[string]float64),
    }
}

func (h *HealthTracker) RecordFailure(channel string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.consecutive[channel]++
    h.totalFail[channel]++
    h.recalc(channel)
}

func (h *HealthTracker) RecordSuccess(channel string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.consecutive[channel] = 0
    h.totalSuccess[channel]++
    h.recalc(channel)
}

func (h *HealthTracker) RecordLatency(channel string, d time.Duration) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.latencies[channel] = append(h.latencies[channel], d)
    if len(h.latencies[channel]) > 100 {
        h.latencies[channel] = h.latencies[channel][1:]
    }
}

func (h *HealthTracker) HealthScore(channel string) float64 {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return h.health[channel]
}

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

func (h *HealthTracker) IsHealthy(channel string) bool {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return h.health[channel] >= 0.5 && h.consecutive[channel] < failureThreshold
}

func (h *HealthTracker) TotalStats(channel string) (fail, success int) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return h.totalFail[channel], h.totalSuccess[channel]
}

func (h *HealthTracker) recalc(channel string) {
    fail := h.totalFail[channel]
    success := h.totalSuccess[channel]
    total := fail + success
    if total == 0 {
        h.health[channel] = maxHealth
        return
    }
    ratio := float64(fail) / float64(total)
    consecutive := h.consecutive[channel]
    h.health[channel] = math.Max(minHealth, math.Min(maxHealth,
        1.0 - (ratio*0.5) - (float64(consecutive)*0.1)))
}
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/health_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/health.go internal/core/health_test.go
git commit -m "feat(core): add HealthTracker with failure tracking and latency recording"
```

---

### Task 4: `core.Router`

**Files:** Create `internal/core/router.go`, Test `internal/core/router_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
    "testing"
)

func TestRouter_WeightedRandom(t *testing.T) {
    reg := NewPluginRegistry()
    ht := NewHealthTracker()
    router := NewRouter(reg, ht)

    ch1 := &mockChannel{name: "ch1"}
    ch2 := &mockChannel{name: "ch2"}
    reg.Register(ch1)
    reg.Register(ch2)

    selected := make(map[string]int)
    for i := 0; i < 1000; i++ {
        ch, err := router.Route()
        if err != nil { t.Fatalf("Route error: %v", err) }
        selected[ch.Name()]++
    }

    if selected["ch1"] == 0 || selected["ch2"] == 0 {
        t.Errorf("both should be selected: ch1=%d ch2=%d", selected["ch1"], selected["ch2"])
    }

    // ch1 unhealthy → should be excluded
    for i := 0; i < 6; i++ {
        ht.RecordFailure("ch1")
    }
    selected = make(map[string]int)
    for i := 0; i < 200; i++ {
        ch, err := router.Route()
        if err != nil { t.Fatalf("Route error when ch1 unhealthy: %v", err) }
        selected[ch.Name()]++
    }
    if selected["ch1"] != 0 {
        t.Errorf("ch1 should not be selected when unhealthy: got %d", selected["ch1"])
    }
}

func TestRouter_RecordResult(t *testing.T) {
    reg := NewPluginRegistry()
    ht := NewHealthTracker()
    router := NewRouter(reg, ht)

    ch := &mockChannel{name: "ch1"}
    reg.Register(ch)

    router.RecordResult("ch1", true, 1500)
    router.RecordResult("ch1", false, 2000)
    router.RecordResult("ch1", false, 3000)

    score := ht.HealthScore("ch1")
    if score >= 0.9 { t.Errorf("health should have dropped: got %f", score) }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/router_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write Router**

```go
package core

import (
    "errors"
    "math/rand"
    "sync"
    "time"
)

// Router selects the best channel using weighted random + health awareness.
type Router struct {
    reg    *PluginRegistry
    health *HealthTracker
    mu     sync.RWMutex
}

func NewRouter(reg *PluginRegistry, health *HealthTracker) *Router {
    return &Router{reg: reg, health: health}
}

// Route selects a healthy channel using weighted random selection.
func (r *Router) Route() (Channel, error) {
    channels := r.reg.List()

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

// RouteForModel routes for a specific model (currently just delegates to Route).
func (r *Router) RouteForModel(modelName string) (Channel, error) {
    return r.Route()
}

// RecordResult updates health based on call outcome.
func (r *Router) RecordResult(channel string, success bool, latencyMs int64) {
    if success {
        r.health.RecordSuccess(channel)
    } else {
        r.health.RecordFailure(channel)
    }
    r.health.RecordLatency(channel, time.Duration(latencyMs*1e6))
}
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/router_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/router.go internal/core/router_test.go
git commit -m "feat(core): add weighted random + health-aware Router"
```

---

### Task 5: `core.JWTIssuer` + `core.JWTMiddleware`

**Files:** Create `internal/core/jwt.go`, Test `internal/core/jwt_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestJWTLifecycle(t *testing.T) {
    issuer := NewJWTIssuer("test-secret", 1*time.Hour)

    claims := &JWTClaims{
        Subject: "user-123",
        APIKey:  "kieai-key-abc",
        Channel: "kieai",
    }
    token, err := issuer.Issue(claims)
    if err != nil { t.Fatalf("Issue error: %v", err) }
    if token == "" { t.Fatal("token should not be empty") }

    parsed, err := issuer.Validate(token)
    if err != nil { t.Fatalf("Validate error: %v", err) }
    if parsed.Subject != "user-123" { t.Errorf("subject mismatch") }
    if parsed.APIKey != "kieai-key-abc" { t.Errorf("apiKey mismatch") }

    // Invalid token
    _, err = issuer.Validate("invalid")
    if err == nil { t.Errorf("expected error for invalid token") }
}

func TestJWTMiddleware_MissingToken(t *testing.T) {
    issuer := NewJWTIssuer("secret", 1*time.Hour)
    var capturedClaims *JWTClaims
    var nextCalled bool

    mw := NewJWTMiddleware(issuer, func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request) {
        capturedClaims = claims
        nextCalled = true
    })

    req := httptest.NewRequest("POST", "/", nil)
    rr := httptest.NewRecorder()
    mw.ServeHTTP(rr, req)

    if rr.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", rr.Code)
    }
    if nextCalled { t.Errorf("next should not be called without token") }
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
    issuer := NewJWTIssuer("secret", 1*time.Hour)
    token, _ := issuer.Issue(&JWTClaims{Subject: "u1", APIKey: "key1"})

    var captured *JWTClaims
    mw := NewJWTMiddleware(issuer, func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request) {
        captured = claims
    })

    req := httptest.NewRequest("POST", "/", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rr := httptest.NewRecorder()
    mw.ServeHTTP(rr, req)

    if !nextCalled || captured == nil || captured.Subject != "u1" {
        t.Errorf("next not called correctly: nextCalled=%v captured=%v", nextCalled, captured)
    }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/jwt_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write JWT implementation**

```go
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
    APIKey  string `json:"api_key,omitempty"`
    Channel string `json:"channel,omitempty"`
    Quota   int64  `json:"quota,omitempty"`
}

// JWTIssuer creates and validates JWTs.
type JWTIssuer struct {
    secret []byte
    expiry time.Duration
}

func NewJWTIssuer(secret string, expiry time.Duration) *JWTIssuer {
    return &JWTIssuer{secret: []byte(secret), expiry: expiry}
}

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

// JWTMiddleware validates incoming JWTs and injects claims into context.
type JWTMiddleware struct {
    issuer *JWTIssuer
    next   func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request)
}

func NewJWTMiddleware(issuer *JWTIssuer, next func(ctx context.Context, claims *JWTClaims, w http.ResponseWriter, r *http.Request)) *JWTMiddleware {
    return &JWTMiddleware{issuer: issuer, next: next}
}

func (m *JWTMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    token := extractBearerToken(r)
    if token == "" {
        writeError(w, http.StatusUnauthorized, "JWT token required", "UNAUTHENTICATED")
        return
    }
    claims, err := m.issuer.Validate(token)
    if err != nil {
        writeError(w, http.StatusUnauthorized, err.Error(), "UNAUTHENTICATED")
        return
    }
    ctx := context.WithValue(r.Context(), contextKeyClaims, claims)
    m.next(ctx, claims, w, r.WithContext(ctx))
}

func extractBearerToken(r *http.Request) string {
    if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return ""
}

func GetClaims(ctx context.Context) *JWTClaims {
    if claims, ok := ctx.Value(contextKeyClaims).(*JWTClaims); ok {
        return claims
    }
    return nil
}

func writeError(w http.ResponseWriter, code int, message, status string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    fmt.Fprintf(w, `{"error":{"code":%d,"message":%q,"status":%q}}`, code, message, status)
}
```

- [ ] **Step 4: Add JWT dependency and run tests**

```bash
cd /Users/botycookie/ai/goloop && go get github.com/golang-jwt/jwt/v5
go mod tidy
go test ./internal/core/jwt_test.go -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/core/jwt.go internal/core/jwt_test.go
git commit -m "feat(core): add JWTIssuer, JWTMiddleware, and JWT claims"
```

---

### Task 6: `core.PluginRegistry`

**Files:** Create `internal/core/pluginRegistry.go`, Test `internal/core/pluginRegistry_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import "testing"

func TestPluginRegistry(t *testing.T) {
    reg := NewPluginRegistry()

    ch1 := &mockChannel{name: "kieai"}
    ch2 := &mockChannel{name: "gemini"}

    reg.Register(ch1)
    reg.Register(ch2)

    ch, err := reg.Get("kieai")
    if err != nil { t.Fatalf("expected kieai: %v", err) }
    if ch.Name() != "kieai" { t.Errorf("name mismatch") }

    _, err = reg.Get("nonexistent")
    if err == nil { t.Errorf("expected error for nonexistent") }

    if len(reg.List()) != 2 { t.Errorf("expected 2 channels") }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/pluginRegistry_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write PluginRegistry**

```go
package core

import (
    "fmt"
    "sync"
)

type PluginRegistry struct {
    mu       sync.RWMutex
    channels map[string]Channel
}

func NewPluginRegistry() *PluginRegistry {
    return &PluginRegistry{channels: make(map[string]Channel)}
}

func (r *PluginRegistry) Register(ch Channel) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if ch == nil { panic("cannot register nil channel") }
    name := ch.Name()
    if name == "" { panic("channel name cannot be empty") }
    if _, exists := r.channels[name]; exists {
        panic(fmt.Sprintf("channel %q already registered", name))
    }
    r.channels[name] = ch
}

func (r *PluginRegistry) Get(name string) (Channel, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    ch, ok := r.channels[name]
    if !ok {
        return nil, fmt.Errorf("pluginRegistry: channel %q not found", name)
    }
    return ch, nil
}

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

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/pluginRegistry_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/pluginRegistry.go internal/core/pluginRegistry_test.go
git commit -m "feat(core): add PluginRegistry for channel lifecycle management"
```

---

## Chunk 2: KIE.AI Channel Plugin

### Task 7: `kieai.AccountPool` — Weighted Random with Health

**Files:** Create `internal/channels/kieai/accountPool.go`, Test `internal/channels/kieai/accountPool_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
        if err != nil { t.Fatalf("Select error: %v", err) }
        selected[acc.APIKey()]++
    }

    if selected["key-1"] <= selected["key-3"] {
        t.Errorf("key-1 should be selected more than key-3: %d vs %d", selected["key-1"], selected["key-3"])
    }

    // Return accounts with failures → mark unhealthy
    all := pool.List()
    acc1 := all[0]
    for i := 0; i < 5; i++ {
        pool.Return(acc1, false)
    }
    if acc1.IsHealthy() { t.Errorf("acc1 should be unhealthy after 5 failures") }

    // Unhealthy should be excluded from selection
    unhealthySelected := 0
    for i := 0; i < 200; i++ {
        acc, _ := pool.Select()
        if acc.APIKey() == acc1.APIKey() {
            unhealthySelected++
        }
    }
    if unhealthySelected > 0 { t.Errorf("unhealthy account should not be selected: got %d", unhealthySelected) }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/accountPool_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write AccountPool**

```go
package kieai

import (
    "errors"
    "math/rand"
    "sync"
)

// kieAccount implements core.Account for the KIE.AI channel.
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
    return 1.0 - float64(a.failCount)*0.2
}
func (a *kieAccount) IsHealthy() bool {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.healthy && a.failCount < 5
}
func (a *kieAccount) IncUsage()          { a.mu.Lock(); defer a.mu.Unlock(); a.usageCount++ }
func (a *kieAccount) RecordFailure()      { a.mu.Lock(); defer a.mu.Unlock(); a.failCount++; if a.failCount >= 5 { a.healthy = false } }
func (a *kieAccount) RecordSuccess()      { a.mu.Lock(); defer a.mu.Unlock(); a.failCount = max(0, a.failCount-1); a.healthy = true }

// AccountPool manages KIE.AI accounts with weighted random selection.
type AccountPool struct {
    mu       sync.RWMutex
    accounts []*kieAccount
}

func NewAccountPool() *AccountPool { return &AccountPool{} }

func (p *AccountPool) AddAccount(apiKey string, weight int) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.accounts = append(p.accounts, &kieAccount{apiKey: apiKey, weight: weight, healthy: true})
}

func (p *AccountPool) Select() (Account, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()

    candidates := make([]Account, 0, len(p.accounts))
    weights := make([]int, 0, len(p.accounts))
    totalWeight := 0

    for _, acc := range p.accounts {
        if acc.IsHealthy() {
            effectiveWeight := acc.Weight() * int(acc.HealthScore()*100) / 100
            if effectiveWeight <= 0 {
                effectiveWeight = 1
            }
            candidates = append(candidates, acc)
            weights = append(weights, effectiveWeight)
            totalWeight += effectiveWeight
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

func (p *AccountPool) Return(acc Account, success bool) {
    acc.RecordSuccess() if success else acc.RecordFailure()
}

func (p *AccountPool) List() []Account {
    p.mu.RLock()
    defer p.mu.RUnlock()
    ret := make([]Account, len(p.accounts))
    for i, a := range p.accounts {
        ret[i] = a
    }
    return ret
}

func (p *AccountPool) Remove(apiKey string) bool {
    p.mu.Lock()
    defer p.mu.Unlock()
    for i, acc := range p.accounts {
        if acc.APIKey() == apiKey {
            p.accounts = append(p.accounts[:i], p.accounts[i+1:]...)
            return true
        }
    }
    return false
}
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/accountPool_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/kieai/accountPool.go internal/channels/kieai/accountPool_test.go
git commit -m "feat(kieai): add weighted random AccountPool with health tracking"
```

---

### Task 8: `kieai.Channel` — Plugin Implementation

**Files:** Create `internal/channels/kieai/channel.go`, Test `internal/channels/kieai/channel_test.go`

- [ ] **Step 1: Write the failing test**

```go
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

    if ch.Name() != "kieai" { t.Errorf("Name mismatch") }
    if !ch.IsAvailable() { t.Errorf("IsAvailable should be true") }

    taskID, err := ch.SubmitTask(context.Background(), "test-key",
        &model.GoogleRequest{Contents: []model.Content{{Parts: []model.Part{{Text: "test"}}}}},
        "gemini-3.1-flash-image-preview")
    if err != nil { t.Fatalf("SubmitTask error: %v", err) }
    if taskID != "task-abc" { t.Errorf("taskID mismatch: got %q", taskID) }

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
    if !ch.Probe(acc) { t.Errorf("Probe should return true when server returns 200") }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/channel_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write KIE.AI Channel**

```go
package kieai

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "net/http"
    "time"

    "goloop/internal/model"
)

// Config holds KIE.AI channel configuration.
type Config struct {
    BaseURL         string
    Timeout         time.Duration
    InitialInterval time.Duration
    MaxInterval     time.Duration
    MaxWaitTime     time.Duration
    RetryAttempts   int
}

func defaultConfig(baseURL string) Config {
    return Config{
        BaseURL:         baseURL,
        Timeout:         120 * time.Second,
        InitialInterval: 2 * time.Second,
        MaxInterval:     10 * time.Second,
        MaxWaitTime:     120 * time.Second,
        RetryAttempts:   3,
    }
}

// Channel implements core.Channel for KIE.AI.
type Channel struct {
    name          string
    baseURL       string
    httpClient    *http.Client
    pool          *AccountPool
    reqTransform  *RequestTransformer
    respTransform *ResponseTransformer
    cfg           Config
}

// Account implements core.Account — the kieAccount already satisfies this.
type Account = core.Account

// NewChannel creates a new KIE.AI channel plugin.
func NewChannel(baseURL string, pool *AccountPool, cfg Config) *Channel {
    if cfg.InitialInterval == 0 {
        cfg = defaultConfig(baseURL)
    }
    ch := &Channel{
        name:       "kieai",
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: cfg.Timeout},
        pool:       pool,
        cfg:        cfg,
    }

    modelMapping := map[string]ModelDefaults{
        "gemini-3.1-flash-image-preview": {KieAIModel: "nano-banana-2", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
        "gemini-3-pro-image-preview":     {KieAIModel: "nano-banana-pro", AspectRatio: "1:1", Resolution: "2K", OutputFormat: "png"},
        "gemini-2.5-flash-image":         {KieAIModel: "google/nano-banana", AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png"},
    }
    ch.reqTransform = NewRequestTransformer(modelMapping)
    ch.respTransform = NewResponseTransformer()
    return ch
}

func (ch *Channel) Name() string          { return ch.name }
func (ch *Channel) IsAvailable() bool    { return ch.pool != nil && len(ch.pool.List()) > 0 }
func (ch *Channel) HealthScore() float64 {
    accounts := ch.pool.List()
    if len(accounts) == 0 { return 0 }
    var total float64
    for _, acc := range accounts {
        total += acc.HealthScore()
    }
    return total / float64(len(accounts))
}

func (ch *Channel) Generate(ctx context.Context, apiKey string, req *model.GoogleRequest, modelName string) (*model.GoogleResponse, error) {
    return nil, fmt.Errorf("kieai: Generate not supported, use SubmitTask + PollTask")
}

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
        return "", err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+apiKey)

    resp, err := ch.httpClient.Do(httpReq)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
    if err != nil {
        return "", err
    }
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("kieai: HTTP %d: %s", resp.StatusCode, string(data))
    }

    var result struct {
        Code int    `json:"code"`
        Data struct {
            TaskID string `json:"taskId"`
        } `json:"data"`
    }
    if err := json.Unmarshal(data, &result); err != nil {
        return "", err
    }
    if result.Data.TaskID == "" {
        return "", fmt.Errorf("kieai: empty taskId")
    }
    return result.Data.TaskID, nil
}

func (ch *Channel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
    deadline := time.Now().Add(ch.cfg.MaxWaitTime)
    interval := ch.cfg.InitialInterval
    consecutiveFails := 0

    for {
        if time.Now().After(deadline) {
            return nil, fmt.Errorf("kieai: task %q timed out", taskID)
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
                return nil, fmt.Errorf("kieai: no result URLs")
            }
            return ch.respTransform.ToGoogleResponse(ctx, record.ResultJSON().ResultURLs)
        case "fail":
            return nil, fmt.Errorf("kieai: task %q failed: %s", taskID, record.FailReason)
        case "waiting", "queuing", "generating":
            interval = min(interval*2, ch.cfg.MaxInterval)
        }
    }
}

func (ch *Channel) getTaskStatus(ctx context.Context, apiKey, taskID string) (*model.KieAIRecordData, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet,
        ch.baseURL+"/api/v1/jobs/recordInfo?taskId="+taskID, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+apiKey)

    resp, err := ch.httpClient.Do(req)
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

// Probe sends a lightweight health check for a specific account.
func (ch *Channel) Probe(account Account) bool {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, http.MethodGet,
        ch.baseURL+"/api/v1/user/info", nil)
    if err != nil {
        return false
    }
    req.Header.Set("Authorization", "Bearer "+account.APIKey())

    resp, err := ch.httpClient.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}
```

Note: `kieai.Channel` references `core.Account` via the `Account` type alias. Add to `channel.go`:
```go
import "goloop/internal/core"
type Account = core.Account
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/channel_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/kieai/channel.go internal/channels/kieai/channel_test.go
git commit -m "feat(kieai): implement core.Channel plugin with SubmitTask, PollTask, and Probe"
```

---

### Task 9: `kieai` Transformers

**Files:** Create `internal/channels/kieai/requestTransformer.go`, `internal/channels/kieai/responseTransformer.go`

- [ ] **Step 1: Write request transformer tests**

```go
package kieai

import (
    "context"
    "testing"

    "goloop/internal/model"
)

func TestRequestTransformer_Transform(t *testing.T) {
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
    if err != nil { t.Fatalf("Transform error: %v", err) }
    if kieReq.Model != "nano-banana-2" { t.Errorf("model: got %q", kieReq.Model) }
    if kieReq.Input.Prompt != "draw a cat" { t.Errorf("prompt: got %q", kieReq.Input.Prompt) }
    if kieReq.Input.AspectRatio != "1:1" { t.Errorf("aspect_ratio mismatch") }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/... -run TestRequestTransformer -v`
Expected: FAIL

- [ ] **Step 3: Write transformers**

```go
// internal/channels/kieai/requestTransformer.go
package kieai

import (
    "context"
    "strings"

    "goloop/internal/model"
)

type ModelDefaults struct {
    KieAIModel   string
    AspectRatio  string
    Resolution   string
    OutputFormat string
}

type RequestTransformer struct {
    modelMapping map[string]ModelDefaults
}

func NewRequestTransformer(mapping map[string]ModelDefaults) *RequestTransformer {
    return &RequestTransformer{modelMapping: mapping}
}

func (t *RequestTransformer) Transform(ctx context.Context, req *model.GoogleRequest, googleModel string) (*model.KieAICreateTaskRequest, error) {
    defaults, ok := t.modelMapping[googleModel]
    if !ok {
        return nil, nil // passthrough to let caller handle
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
        if ic.AspectRatio != ""  { input.AspectRatio = ic.AspectRatio }
        if ic.ImageSize != ""    { input.Resolution = ic.ImageSize }
        if ic.OutputFormat != "" { input.OutputFormat = ic.OutputFormat }
    }

    return &model.KieAICreateTaskRequest{
        Model: defaults.KieAIModel,
        Input: input,
    }, nil
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

    type imgResult struct{ idx int; data []byte; err error }
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
                ch <- imgResult{idx: idx}
            }
        }(i, url)
    }

    go func() { wg.Wait(); close(ch) }()
    for r := range ch { results[r.idx] = r }

    parts := []model.Part{{Text: fmt.Sprintf("Generated %d image(s) successfully.", len(resultURLs))}}
    for _, r := range results {
        if r.err != nil { return nil, fmt.Errorf("download image %d: %w", r.idx, r.err) }
        parts = append(parts, model.Part{
            InlineData: &model.InlineData{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(r.data)},
        })
    }

    return &model.GoogleResponse{
        Candidates: []model.Candidate{
            {Content: model.Content{Parts: parts}, FinishReason: "STOP"},
        },
    }, nil
}
```

- [ ] **Step 4: Run tests — verify they pass**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/channels/kieai/... -v -timeout 30s`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channels/kieai/requestTransformer.go internal/channels/kieai/responseTransformer.go
git commit -m "feat(kieai): add request and response transformers"
```

---

## Chunk 3: Handler, Config, Admin API, and Server Wiring

### Task 10: Handler — Refactor to Use Core

**Files:** Modify `internal/handler/gemini_handler.go`

- [ ] **Step 1: Read current handler to understand scope**

```bash
cat internal/handler/gemini_handler.go
```

- [ ] **Step 2: Rewrite handler to delegate to core.Router**

Key changes:
- Remove direct `kieai.Client` / `kieai.Poller` / `kieai.TaskManager` references
- Add `router *core.Router`, `registry *core.PluginRegistry`, `issuer *core.JWTIssuer`, `storage *storage.Store` fields
- `RegisterRoutes`: mount JWT-protected route via `core.NewJWTMiddleware`
- `handleProtected`: after JWT validation, call `router.Route()`, then `channel.SubmitTask()` + `channel.PollTask()`, then `router.RecordResult()`
- Preserve existing API surface (same paths, same JSON format)
- Preserve SSE streaming path if already implemented

```go
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
    "time"

    "goloop/internal/core"
    "goloop/internal/model"
    "goloop/internal/storage"
)

const maxRequestBodyBytes = 10 * 1024 * 1024

type GeminiHandler struct {
    router   *core.Router
    registry *core.PluginRegistry
    issuer   *core.JWTIssuer
    storage  *storage.Store
}

func NewGeminiHandler(router *core.Router, registry *core.PluginRegistry, issuer *core.JWTIssuer, storage *storage.Store) *GeminiHandler {
    return &GeminiHandler{router: router, registry: registry, issuer: issuer, storage: storage}
}

func (h *GeminiHandler) RegisterRoutes(mux *http.ServeMux) {
    // JWT-protected POST /v1beta/models/{model}:generateContent
    protected := core.NewJWTMiddleware(h.issuer, h.handleProtected)
    mux.Handle("POST /v1beta/models/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        protected.ServeHTTP(w, r)
    }))

    mux.HandleFunc("GET /v1beta/models", h.handleListModels)
    mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *GeminiHandler) handleProtected(ctx context.Context, claims *core.JWTClaims, w http.ResponseWriter, r *http.Request) {
    googleModel := extractModel(r.URL.Path)
    if googleModel == "" {
        writeGoogleError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
        return
    }

    requestID := generateRequestID()
    log := slog.With("requestId", requestID, "googleModel", googleModel)

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

    ch, err := h.router.RouteForModel(googleModel)
    if err != nil {
        log.Error("no channel available", "err", err)
        writeGoogleError(w, http.StatusServiceUnavailable, "no channel available", "INTERNAL")
        return
    }

    apiKey := claims.APIKey
    if apiKey == "" {
        apiKey = extractAPIKey(r)
    }

    start := time.Now()

    taskID, err := ch.SubmitTask(ctx, apiKey, &googleReq, googleModel)
    if err != nil {
        log.Error("submit failed", "channel", ch.Name(), "err", err)
        h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
        writeGoogleError(w, http.StatusInternalServerError, err.Error(), "INTERNAL")
        return
    }

    resp, err := ch.PollTask(ctx, apiKey, taskID)
    if err != nil {
        log.Error("poll failed", "channel", ch.Name(), "err", err)
        h.router.RecordResult(ch.Name(), false, time.Since(start).Milliseconds())
        writeGoogleError(w, http.StatusInternalServerError, err.Error(), "INTERNAL")
        return
    }

    h.router.RecordResult(ch.Name(), true, time.Since(start).Milliseconds())
    log.Info("completed", "channel", ch.Name(), "latency", time.Since(start))

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
}

func (h *GeminiHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
    models := []string{"gemini-3.1-flash-image-preview", "gemini-3-pro-image-preview", "gemini-2.5-flash-image"}
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
    if !found { return "" }
    return model
}

func readBody(r *http.Request) ([]byte, error) {
    limited := io.LimitReader(r.Body, maxRequestBodyBytes+1)
    data, err := io.ReadAll(limited)
    if err != nil { return nil, err }
    if len(data) > maxRequestBodyBytes { return nil, errors.New("request body too large") }
    return data, nil
}

func extractAPIKey(r *http.Request) string {
    if k := r.Header.Get("x-goog-api-key"); k != "" { return k }
    if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return ""
}

func writeGoogleError(w http.ResponseWriter, code int, message, status string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(model.GoogleError{Error: model.GoogleErrorDetail{Code: code, Message: message, Status: status}})
}

func generateRequestID() string {
    b := make([]byte, 8)
    rand.Read(b)
    return hex.EncodeToString(b)
}
```

- [ ] **Step 3: Build and test**

```bash
cd /Users/botycookie/ai/goloop && go build ./...
go test ./internal/handler/... -v -timeout 30s
```
Expected: PASS (existing tests should still pass after stripping kieai direct deps)

- [ ] **Step 4: Commit**

```bash
git add internal/handler/gemini_handler.go
git commit -m "refactor(handler): delegate to core.Router, remove direct kieai client references"
```

---

### Task 11: Config — Multi-Channel Configuration

**Files:** Modify `internal/config/config.go`, Create `config/config.yaml`

- [ ] **Step 1: Update Config struct**

Replace the existing `config.go` (which currently has single-channel `KieAIConfig`) with:

```go
package config

import (
    "fmt"
    "os"
    "strconv"
    "time"
)

type Config struct {
    Server  ServerConfig
    JWT     JWTConfig
    Storage StorageConfig
    Health  HealthConfig
    Channels map[string]ChannelConfig
    ModelMapping map[string]ModelDefaults
}

type ServerConfig struct {
    Port         int
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}

type JWTConfig struct {
    Secret string
    Expiry time.Duration
}

type StorageConfig struct {
    Type      string
    LocalPath string
    BaseURL   string
}

type HealthConfig struct {
    ProbeInterval     time.Duration
    ProbeTimeout     time.Duration
    RecoveryThreshold int
}

type ChannelConfig struct {
    Type            string
    BaseURL         string
    Timeout         time.Duration
    InitialInterval time.Duration
    MaxInterval     time.Duration
    MaxWaitTime     time.Duration
    RetryAttempts   int
    Accounts        []AccountConfig
}

type AccountConfig struct {
    APIKey string
    Weight int
}

type ModelDefaults struct {
    Channel      string
    KieAIModel  string
    AspectRatio  string
    Resolution   string
    OutputFormat string
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" { return v }
    return fallback
}

func getEnvDuration(key, fallback string) time.Duration {
    d, err := time.ParseDuration(getEnv(key, fallback))
    if err != nil { d, _ = time.ParseDuration(fallback) }
    return d
}

func getEnvInt(key string, fallback int) int {
    s := os.Getenv(key)
    if s == "" { return fallback }
    n, err := strconv.Atoi(s)
    if err != nil { return fallback }
    return n
}

// Load reads from environment variables. Use YAML file for structured config.
func Load() (*Config, error) {
    jwtSecret := getEnv("JWT_SECRET", "")
    if jwtSecret == "" {
        return nil, fmt.Errorf("config: JWT_SECRET is required")
    }

    cfg := &Config{
        Server: ServerConfig{
            Port:         getEnvInt("SERVER_PORT", 8080),
            ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", "130s"),
            WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", "130s"),
        },
        JWT: JWTConfig{
            Secret: jwtSecret,
            Expiry: getEnvDuration("JWT_EXPIRY", "24h"),
        },
        Storage: StorageConfig{
            Type:      getEnv("STORAGE_TYPE", "local"),
            LocalPath: getEnv("STORAGE_LOCAL_PATH", "/tmp/images"),
            BaseURL:   getEnv("STORAGE_BASE_URL", ""),
        },
        Health: HealthConfig{
            ProbeInterval:     getEnvDuration("HEALTH_PROBE_INTERVAL", "30s"),
            ProbeTimeout:     getEnvDuration("HEALTH_PROBE_TIMEOUT", "5s"),
            RecoveryThreshold: getEnvInt("HEALTH_RECOVERY_THRESHOLD", 2),
        },
        ModelMapping: map[string]ModelDefaults{
            "gemini-3.1-flash-image-preview": {
                Channel:     "kieai",
                KieAIModel: getEnv("MODEL_NANO_BANANA_2", "nano-banana-2"),
                AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
            },
            "gemini-3-pro-image-preview": {
                Channel:     "kieai",
                KieAIModel: getEnv("MODEL_NANO_BANANA_PRO", "nano-banana-pro"),
                AspectRatio: "1:1", Resolution: "2K", OutputFormat: "png",
            },
            "gemini-2.5-flash-image": {
                Channel:     "kieai",
                KieAIModel: getEnv("MODEL_GOOGLE_NANO_BANANA", "google/nano-banana"),
                AspectRatio: "1:1", Resolution: "1K", OutputFormat: "png",
            },
        },
    }

    // Build channel configs from env (for simple deployment)
    // KIE.AI channel
    kieBaseURL := getEnv("KIEAI_BASE_URL", "")
    if kieBaseURL != "" {
        cfg.Channels = map[string]ChannelConfig{
            "kieai": {
                Type:            "kieai",
                BaseURL:         kieBaseURL,
                Timeout:         getEnvDuration("KIEAI_TIMEOUT", "120s"),
                InitialInterval: getEnvDuration("POLLER_INITIAL_INTERVAL", "2s"),
                MaxInterval:     getEnvDuration("POLLER_MAX_INTERVAL", "10s"),
                MaxWaitTime:    getEnvDuration("POLLER_MAX_WAIT_TIME", "120s"),
                RetryAttempts:   getEnvInt("POLLER_RETRY_ATTEMPTS", 3),
            },
        }
        // Read account keys from env: KIEAI_KEY_1, KIEAI_KEY_2, ...
        for i := 1; i <= 10; i++ {
            key := os.Getenv(fmt.Sprintf("KIEAI_KEY_%d", i))
            if key == "" { continue }
            weight := getEnvInt(fmt.Sprintf("KIEAI_WEIGHT_%d", i), 100)
            cfg.Channels["kieai"].Accounts = append(cfg.Channels["kieai"].Accounts,
                AccountConfig{APIKey: key, Weight: weight})
        }
    }

    return cfg, nil
}

func validate(cfg *Config) error {
    if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
        return fmt.Errorf("invalid SERVER_PORT %d", cfg.Server.Port)
    }
    if cfg.JWT.Secret == "" {
        return fmt.Errorf("config: JWT_SECRET is required")
    }
    return nil
}
```

- [ ] **Step 2: Write `config/config.yaml`**

```yaml
server:
  port: 8080
  read_timeout: 130s
  write_timeout: 130s

jwt:
  secret: "${JWT_SECRET}"
  expiry: 24h

storage:
  type: local
  local_path: /tmp/images
  base_url: ${STORAGE_BASE_URL}

health:
  probe_interval: 30s
  probe_timeout: 5s
  recovery_threshold: 2

channels:
  kieai:
    type: kieai
    base_url: https://api.kie.ai
    timeout: 120s
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

model_mapping:
  gemini-3.1-flash-image-preview:
    channel: kieai
    kieai_model: nano-banana-2
    aspect_ratio: "1:1"
    resolution: "1K"
    output_format: png
```

- [ ] **Step 3: Build and test**

```bash
cd /Users/botycookie/ai/goloop && go build ./... && go test ./internal/config/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go config/config.yaml
git commit -m "feat(config): add multi-channel, multi-account configuration"
```

---

### Task 12: `core.ChannelHealthReaper`

**Files:** Create `internal/core/healthReaper.go`, Test `internal/core/healthReaper_test.go`

- [ ] **Step 1: Write the failing test**

```go
package core

import (
    "testing"
    "time"
)

func TestHealthReaper_DoesNotProbeHealthy(t *testing.T) {
    reg := NewPluginRegistry()
    ht := NewHealthTracker()
    reaper := NewHealthReaper(reg, ht, 1*time.Second, 2)

    ch := &mockChannel{name: "ch1"}
    reg.Register(ch)

    // Wait 2 probe cycles
    time.Sleep(2500 * time.Millisecond)

    // reaper should not panic and channel should remain healthy
    if !ht.IsHealthy("ch1") { t.Errorf("healthy channel should stay healthy") }
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/healthReaper_test.go -v`
Expected: FAIL

- [ ] **Step 3: Write HealthReaper**

```go
package core

import (
    "context"
    "log/slog"
    "sync"
    "time"
)

// ChannelHealthReaper periodically probes unhealthy accounts to recover them.
type ChannelHealthReaper struct {
    reg             *PluginRegistry
    health          *HealthTracker
    probeInterval   time.Duration
    recoveryThresh int
    stopCh          chan struct{}
    wg              sync.WaitGroup
}

// NewHealthReaper creates a reaper that starts probing unhealthy accounts.
func NewHealthReaper(reg *PluginRegistry, health *HealthTracker, interval time.Duration, recoveryThresh int) *ChannelHealthReaper {
    return &ChannelHealthReaper{
        reg:             reg,
        health:          health,
        probeInterval:   interval,
        recoveryThresh: recoveryThresh,
        stopCh:          make(chan struct{}),
    }
}

// Start begins the background probe loop.
func (r *ChannelHealthReaper) Start() {
    r.wg.Add(1)
    go r.run()
    slog.Info("healthReaper started", "interval", r.probeInterval)
}

func (r *ChannelHealthReaper) run() {
    defer r.wg.Done()
    ticker := time.NewTicker(r.probeInterval)
    defer ticker.Stop()

    for {
        select {
        case <-r.stopCh:
            slog.Info("healthReaper stopped")
            return
        case <-ticker.C:
            r.probeUnhealthyAccounts()
        }
    }
}

// Stop gracefully stops the reaper.
func (r *ChannelHealthReaper) Stop() {
    close(r.stopCh)
    r.wg.Wait()
}

func (r *ChannelHealthReaper) probeUnhealthyAccounts() {
    for _, ch := range r.reg.List() {
        accounts := getAccounts(ch)
        for _, acc := range accounts {
            if acc.IsHealthy() {
                continue // skip healthy accounts
            }

            // Probe the account
            ok := ch.Probe(acc)
            if ok {
                // Probe succeeded: record as success (not counted as failure)
                r.health.RecordSuccess(ch.Name())
                slog.Info("healthReaper: probe succeeded, recovered",
                    "channel", ch.Name(), "apiKey", acc.APIKey()[:8]+"...")
            } else {
                // Probe failed: just log, do not penalize
                slog.Debug("healthReaper: probe failed",
                    "channel", ch.Name(), "apiKey", acc.APIKey()[:8]+"...")
            }
        }
    }
}

// getAccounts extracts accounts from a channel.
// If the channel has an AccountPool, use it; otherwise return nil.
func getAccounts(ch Channel) []Account {
    // Type assertion to get AccountPool if the channel exposes it
    // This avoids a hard dependency from core on a specific pool impl
    // In practice, channels implementing a GetAccountPool() interface would be used.
    // For now, channels expose accounts through their own internal pool.
    return nil // will be implemented via interface extension
}
```

**Note**: The `getAccounts` approach above requires a cleaner abstraction. Add to `channel.go`:

```go
// ChannelWithPool is implemented by channels that expose an AccountPool.
type ChannelWithPool interface {
    Channel
    GetAccountPool() AccountPool
}
```

Then `probeUnhealthyAccounts` uses type assertion.

- [ ] **Step 4: Run test — verify it passes**

Run: `cd /Users/botycookie/ai/goloop && go test ./internal/core/healthReaper_test.go -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/core/healthReaper.go internal/core/healthReaper_test.go
git commit -m "feat(core): add ChannelHealthReaper for background probe and auto-recovery"
```

---

### Task 13: Admin API Handler

**Files:** Create `internal/handler/admin_handler.go`, Test `internal/handler/admin_handler_test.go`

- [ ] **Step 1: Write the failing test**

```go
package handler

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestAdminIssueToken(t *testing.T) {
    // Wire a minimal admin handler
    // This test verifies the issue-token endpoint works
    // Full test requires actual issuer, router, registry wiring
}
```

- [ ] **Step 2: Write Admin API handler**

```go
package handler

import (
    "encoding/json"
    "net/http"

    "goloop/internal/core"
    "goloop/internal/model"
)

// AdminHandler provides administrative operations.
type AdminHandler struct {
    issuer   *core.JWTIssuer
    registry *core.PluginRegistry
    health   *core.HealthTracker
}

func NewAdminHandler(issuer *core.JWTIssuer, registry *core.PluginRegistry, health *core.HealthTracker) *AdminHandler {
    return &AdminHandler{issuer: issuer, registry: registry, health: health}
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /admin/issue-token", h.handleIssueToken)
    mux.HandleFunc("GET /admin/stats", h.handleStats)
    mux.HandleFunc("GET /admin/channel/", h.handleChannelAccounts)
    mux.HandleFunc("POST /admin/channel/", h.handleChannelOp)
}

func (h *AdminHandler) handleIssueToken(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Subject string `json:"subject"`
        APIKey  string `json:"api_key"`
        Channel string `json:"channel"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSONError(w, http.StatusBadRequest, err.Error())
        return
    }
    claims := &core.JWTClaims{
        Subject: req.Subject,
        APIKey:  req.APIKey,
        Channel: req.Channel,
    }
    token, err := h.issuer.Issue(claims)
    if err != nil {
        writeJSONError(w, http.StatusInternalServerError, err.Error())
        return
    }
    json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
    stats := make(map[string]any)
    for _, ch := range h.registry.List() {
        fail, success := h.health.TotalStats(ch.Name())
        stats[ch.Name()] = map[string]any{
            "health_score": h.health.HealthScore(ch.Name()),
            "is_healthy":   h.health.IsHealthy(ch.Name()),
            "total_fail":   fail,
            "total_success": success,
            "avg_latency_ms": h.health.AverageLatency(ch.Name()).Milliseconds(),
        }
    }
    json.NewEncoder(w).Encode(stats)
}

func (h *AdminHandler) handleChannelAccounts(w http.ResponseWriter, r *http.Request) {
    // GET /admin/channel/{channel}/accounts
    // Returns list of accounts with status
    channelName := strings.TrimPrefix(r.URL.Path, "/admin/channel/")
    channelName = strings.TrimSuffix(channelName, "/accounts")

    ch, err := h.registry.Get(channelName)
    if err != nil {
        writeJSONError(w, http.StatusNotFound, err.Error())
        return
    }

    // Get accounts via ChannelWithPool interface
    if chwp, ok := ch.(interface{ ListAccounts() []interface{} }); ok {
        json.NewEncoder(w).Encode(chwp.ListAccounts())
        return
    }
    json.NewEncoder(w).Encode([]any{})
}

func (h *AdminHandler) handleChannelOp(w http.ResponseWriter, r *http.Request) {
    // POST /admin/channel/{channel}/accounts/{op}
    // ops: reset, retire, add
    path := r.URL.Path
    // Parse channel name and operation from path
    // Route to appropriate handler
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(model.GoogleError{
        Error: model.GoogleErrorDetail{Code: code, Message: message, Status: "INVALID_ARGUMENT"},
    })
}
```

- [ ] **Step 3: Build and test**

```bash
cd /Users/botycookie/ai/goloop && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/handler/admin_handler.go
git commit -m "feat(admin): add Admin API handler for token issuance and channel management"
```

---

### Task 14: Server — Wire Everything Together

**Files:** Modify `cmd/server/main.go`

- [ ] **Step 1: Write the wired main.go**

```go
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

    // Core infrastructure
    registry := core.NewPluginRegistry()
    health := core.NewHealthTracker()
    router := core.NewRouter(registry, health)
    issuer := core.NewJWTIssuer(cfg.JWT.Secret, cfg.JWT.Expiry)

    // Storage
    store, err := storage.NewStore(cfg.Storage.LocalPath, cfg.Storage.BaseURL)
    if err != nil {
        slog.Error("failed to init storage", "err", err)
        os.Exit(1)
    }

    // Bootstrap channels
    for name, chCfg := range cfg.Channels {
        switch chCfg.Type {
        case "kieai":
            pool := kieai.NewAccountPool()
            for _, acc := range chCfg.Accounts {
                pool.AddAccount(acc.APIKey, acc.Weight)
            }
            kieCh := kieai.NewChannel(chCfg.BaseURL, pool, kieai.Config{
                BaseURL:         chCfg.BaseURL,
                InitialInterval: chCfg.InitialInterval,
                MaxInterval:     chCfg.MaxInterval,
                MaxWaitTime:    chCfg.MaxWaitTime,
                RetryAttempts:   chCfg.RetryAttempts,
            })
            registry.Register(kieCh)
            slog.Info("channel registered", "name", name, "accounts", len(chCfg.Accounts))
        default:
            slog.Warn("unknown channel type", "name", name, "type", chCfg.Type)
        }
    }

    if len(registry.List()) == 0 {
        slog.Error("no channels registered")
        os.Exit(1)
    }

    // Health reaper
    reaper := core.NewHealthReaper(registry, health, cfg.Health.ProbeInterval, cfg.Health.RecoveryThreshold)
    reaper.Start()

    // Handlers
    geminiHandler := handler.NewGeminiHandler(router, registry, issuer, store)
    adminHandler := handler.NewAdminHandler(issuer, registry, health)

    mux := http.NewServeMux()
    geminiHandler.RegisterRoutes(mux)
    adminHandler.RegisterRoutes(mux)
    mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(cfg.Storage.LocalPath))))
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.Redirect(w, r, "/admin/", http.StatusFound)
    })

    // Admin UI static files
    mux.HandleFunc("/admin/ui/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "internal/admin/ui/index.html")
    })

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
    reaper.Stop()

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    server.Shutdown(ctx)
    slog.Info("server stopped")
}
```

- [ ] **Step 2: Build and smoke test**

```bash
cd /Users/botycookie/ai/goloop && go build ./...
go test ./... -timeout 60s
```

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(server): wire core plugin system, multi-channel router, health reaper, and admin API"
```

---

## Chunk 4: Admin UI (Static Files)

### Task 15: Admin UI — HTML + CSS + JS

**Files:** Create `internal/admin/ui/index.html`, `internal/admin/ui/styles.css`, `internal/admin/ui/app.js`

- [ ] **Step 1: Write Admin UI**

Admin UI is a single-page application served as static files. No build step required — files are embedded via `go:embed` or served directly from disk.

**`internal/admin/ui/index.html`** — SPA shell:
```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>goloop Admin Console</title>
  <link rel="stylesheet" href="/admin/ui/styles.css">
</head>
<body>
  <div id="app">
    <aside class="sidebar">
      <div class="logo">🛰 goloop</div>
      <nav class="nav">
        <a href="#dashboard" class="nav-item active" data-page="dashboard">📊 概览</a>
        <a href="#channels" class="nav-item" data-page="channels">📡 渠道管理</a>
        <a href="#accounts" class="nav-item" data-page="accounts">👤 账号池</a>
        <a href="#tools" class="nav-item" data-page="tools">🔧 工具</a>
        <a href="#stats" class="nav-item" data-page="stats">📈 统计</a>
      </nav>
      <div class="sidebar-footer">goloop v0.2</div>
    </aside>

    <main class="content">
      <!-- Dashboard page -->
      <div id="page-dashboard" class="page active">
        <h1>概览</h1>
        <div class="stats-grid" id="stats-grid"></div>
        <div class="section">
          <h2>渠道健康状态</h2>
          <div class="channel-cards" id="channel-cards"></div>
        </div>
        <div class="section">
          <h2>24 小时趋势</h2>
          <canvas id="trend-chart" width="800" height="120"></canvas>
        </div>
      </div>

      <!-- Accounts page -->
      <div id="page-accounts" class="page">
        <h1>账号池</h1>
        <div class="section">
          <div class="section-header">
            <h2>账号列表 — <span id="channel-name">KIE.AI</span></h2>
            <button class="btn btn-primary" onclick="showAddAccount()">+ 添加账号</button>
          </div>
          <table class="account-table">
            <thead>
              <tr>
                <th>账号ID</th><th>API Key</th><th>权重</th>
                <th>状态</th><th>累计请求</th><th>成功率</th>
                <th>连续失败</th><th>最后使用</th><th>操作</th>
              </tr>
            </thead>
            <tbody id="accounts-tbody"></tbody>
          </table>
        </div>
      </div>

      <!-- Tools page -->
      <div id="page-tools" class="page">
        <h1>工具</h1>
        <div class="panel">
          <h3>颁发 JWT Token</h3>
          <form id="issue-token-form" onsubmit="issueToken(event)">
            <div class="form-group">
              <label>Subject (sub)</label>
              <input type="text" name="subject" placeholder="user-123" required>
            </div>
            <div class="form-group">
              <label>API Key</label>
              <input type="text" name="api_key" placeholder="kie_xxxxxxxx" required>
            </div>
            <div class="form-group">
              <label>Channel</label>
              <select name="channel">
                <option value="">不限制</option>
                <option value="kieai">KIE.AI</option>
              </select>
            </div>
            <button type="submit" class="btn btn-primary">生成 Token</button>
          </form>
          <div id="token-result" class="token-result"></div>
        </div>
        <div class="panel">
          <h3>批量操作</h3>
          <button class="btn" onclick="probeAll()">🔍 探测所有 Unhealthy 账号</button>
          <button class="btn" onclick="refreshAll()">🔄 刷新所有状态</button>
        </div>
      </div>
    </main>
  </div>
  <script src="/admin/ui/app.js"></script>
</body>
</html>
```

**`internal/admin/ui/styles.css`** — Minimal dark theme:
```css
:root {
  --bg: #0d1117; --card: #161b22; --border: #30363d;
  --text: #e6edf3; --text2: #8b949e; --text3: #6e7681;
  --green: #2ea043; --orange: #d29922; --red: #f85149;
  --blue: #58a6ff; --purple: #bc8cff;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body { background: var(--bg); color: var(--text); font-family: 'IBM Plex Mono', monospace; font-size: 13px; }
#app { display: flex; height: 100vh; }
.sidebar { width: 220px; background: var(--card); border-right: 1px solid var(--border); display: flex; flex-direction: column; }
.logo { padding: 20px; font-size: 16px; font-weight: bold; border-bottom: 1px solid var(--border); }
.nav { flex: 1; padding: 10px 0; }
.nav-item { display: block; padding: 10px 20px; color: var(--text2); text-decoration: none; }
.nav-item:hover, .nav-item.active { background: rgba(255,255,255,0.05); color: var(--text); }
.content { flex: 1; padding: 30px; overflow-y: auto; }
.page { display: none; }
.page.active { display: block; }
h1 { font-size: 20px; margin-bottom: 24px; }
.stats-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 32px; }
.stat-card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 16px; }
.stat-card .label { color: var(--text2); font-size: 11px; margin-bottom: 8px; }
.stat-card .value { font-size: 24px; font-weight: bold; }
.stat-card .sub { color: var(--text3); font-size: 10px; margin-top: 4px; }
.channel-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-bottom: 32px; }
.channel-card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 16px; }
.channel-card .name { font-size: 14px; font-weight: bold; margin-bottom: 8px; }
.channel-card .status { display: flex; align-items: center; gap: 6px; font-size: 12px; }
.channel-card .online { color: var(--green); }
.channel-card .degraded { color: var(--orange); }
.channel-card .offline { color: var(--red); }
.account-table { width: 100%; border-collapse: collapse; }
.account-table th { text-align: left; padding: 8px 12px; color: var(--text2); font-size: 11px; border-bottom: 1px solid var(--border); }
.account-table td { padding: 10px 12px; border-bottom: 1px solid var(--border); font-size: 12px; }
.account-table tr:hover { background: rgba(255,255,255,0.02); }
.badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 10px; }
.badge-healthy { background: rgba(46,160,67,0.2); color: var(--green); }
.badge-degraded { background: rgba(210,153,34,0.2); color: var(--orange); }
.badge-unhealthy { background: rgba(248,81,73,0.2); color: var(--red); }
.btn { display: inline-block; padding: 6px 14px; border-radius: 6px; border: 1px solid var(--border); background: transparent; color: var(--text2); cursor: pointer; font-size: 12px; }
.btn:hover { border-color: var(--text2); color: var(--text); }
.btn-primary { background: var(--blue); border-color: var(--blue); color: white; }
.panel { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 20px; margin-bottom: 20px; }
.panel h3 { margin-bottom: 16px; font-size: 14px; }
.form-group { margin-bottom: 12px; }
.form-group label { display: block; color: var(--text2); font-size: 11px; margin-bottom: 4px; }
.form-group input, .form-group select { width: 100%; padding: 8px 12px; background: var(--bg); border: 1px solid var(--border); border-radius: 4px; color: var(--text); font-size: 12px; }
.token-result { margin-top: 16px; padding: 12px; background: var(--bg); border-radius: 4px; word-break: break-all; font-size: 11px; }
```

**`internal/admin/ui/app.js`** — API integration:
```javascript
const API_BASE = '';

async function api(path, opts = {}) {
  const res = await fetch(API_BASE + path, opts);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function loadDashboard() {
  const stats = await api('/admin/stats');
  renderStats(stats);
  const channels = await api('/admin/channel/kieai/accounts');
  renderChannelCards(channels);
}

async function issueToken(e) {
  e.preventDefault();
  const fd = new FormData(e.target);
  const body = Object.fromEntries(fd);
  const result = await api('/admin/issue-token', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body),
  });
  document.getElementById('token-result').textContent = result.token;
}

async function probeAccount(channel, id) {
  await api(`/admin/channel/${channel}/accounts/${id}/probe`, {method: 'POST'});
  loadDashboard();
}

async function resetAccount(channel, id) {
  await api(`/admin/channel/${channel}/accounts/${id}/reset`, {method: 'POST'});
  loadDashboard();
}

async function retireAccount(channel, id) {
  if (!confirm('确认下线该账号？')) return;
  await api(`/admin/channel/${channel}/accounts/${id}/retire`, {method: 'POST'});
  loadDashboard();
}

function renderStats(stats) {
  const grid = document.getElementById('stats-grid');
  if (!grid) return;
  grid.innerHTML = Object.entries(stats).map(([name, s]) => `
    <div class="stat-card">
      <div class="label">${name}</div>
      <div class="value" style="color:${s.is_healthy ? 'var(--green)' : 'var(--red)'}">${(s.health_score * 100).toFixed(0)}%</div>
      <div class="sub">${s.total_success} 成功 / ${s.total_fail} 失败 | ${s.avg_latency_ms}ms</div>
    </div>
  `).join('');
}

function renderChannelCards(accounts) {
  const container = document.getElementById('channel-cards');
  if (!container) return;
  container.innerHTML = (accounts || []).map(acc => `
    <div class="channel-card">
      <div class="name">${acc.id}</div>
      <div class="status ${acc.status}">
        ${acc.status === 'healthy' ? '●' : acc.status === 'unhealthy' ? '○' : '◐'}
        ${acc.status}
      </div>
      <div style="margin-top:8px;font-size:11px;color:var(--text2)">
        请求: ${acc.usage_count} | 成功率: ${acc.success_rate}%
      </div>
      <div style="margin-top:12px">
        <button class="btn" onclick="probeAccount('kieai','${acc.id}')">探测</button>
        <button class="btn" onclick="resetAccount('kieai','${acc.id}')">重置</button>
        <button class="btn" onclick="retireAccount('kieai','${acc.id}')">下线</button>
      </div>
    </div>
  `).join('');
}

// Page routing
document.querySelectorAll('.nav-item').forEach(el => {
  el.addEventListener('click', e => {
    e.preventDefault();
    const page = el.dataset.page;
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    document.getElementById('page-' + page)?.classList.add('active');
    el.classList.add('active');
    if (page === 'dashboard') loadDashboard();
  });
});

loadDashboard();
```

- [ ] **Step 2: Verify files exist**

```bash
ls -la internal/admin/ui/
```

- [ ] **Step 3: Commit**

```bash
git add internal/admin/ui/index.html internal/admin/ui/styles.css internal/admin/ui/app.js
git commit -m "feat(admin): add Admin UI (HTML+CSS+JS, no build step)"
```

---

## Implementation Order

```
Chunk 1 — Core Interfaces (6 Tasks):
  Task 1  Channel interface
  Task 2  Account + AccountPool interfaces
  Task 3  HealthTracker
  Task 4  Router
  Task 5  JWTIssuer + JWTMiddleware  ← needs go get jwt/v5
  Task 6  PluginRegistry

Chunk 2 — KIE.AI Channel (3 Tasks):
  Task 7  kieai.AccountPool
  Task 8  kieai.Channel (SubmitTask + PollTask + Probe)
  Task 9  kieai Transformers

Chunk 3 — Wiring (5 Tasks):
  Task 10 Handler refactor → core.Router
  Task 11 Config (multi-channel YAML + env)
  Task 12 ChannelHealthReaper
  Task 13 Admin API handler
  Task 14 main.go wiring

Chunk 4 — Admin UI (1 Task):
  Task 15 Admin UI static files
```

## Key Design Decisions

1. **core.Account as interface** — Each channel implements `core.Account` its own way; `kieAccount` in kieai package satisfies it
2. **Account type alias** — `type Account = core.Account` in each channel package avoids import cycles
3. **Health probe isolation** — Reaper probes only unhealthy accounts, failures don't compound, uses `channel.Probe(account)` not a generic URL
4. **Admin UI is static files** — Served directly by the server, no embedding needed (or use `go:embed` for single-binary convenience)
5. **JWT api_key field** — Token carries the per-user credential so the handler never needs to look up keys
6. **Channel as account owner** — Each channel owns its AccountPool; core Router only sees `Channel.HealthScore()`
