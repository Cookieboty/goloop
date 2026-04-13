# Subrouter 渠道实施计划

> **工程师必读：** 请使用 superpowers:subagent-driven-development 执行本计划。步骤使用复选框（`- [ ]`）语法标记进度。

**目标：** 新增 subrouter 渠道插件，对外接口（`/v1beta/models/{model}:generateContent`）与 kieai 完全一致，内部将 Google 请求格式转为 OpenAI 格式调用上游，上游响应（流式/非流式）再转回 Google 格式。

**架构：**
- 对外接口：复用现有 `/v1beta/models/{model}:generateContent`，无需新建 Handler
- 路由：`Router.RouteForModel` 按 `ModelDefaults.Channel = "subrouter"` 选渠道
- 请求转换：`SubmitTask` 内部将 `model.GoogleRequest` 转为 `OpenAI ChatRequest`
- 响应转换：非流式 → `model.GoogleResponse`；流式 → 逐块 `SSE` 转为 Google SSE
- 账号池复用 `kieai.AccountPool`
- 健康分通过 `router.RecordResult` 更新，无需改动 `HealthTracker`

---

## Chunk 1：Subrouter Channel 核心

### Task 1：创建 `internal/channels/subrouter/channel.go`

**文件：**
- 新建：`internal/channels/subrouter/channel.go`

- [ ] **Step 1：确认 kieai.AccountPool 可被复用**

kieai 的 `AccountPool` 是否导出了 `NewAccountPool`、`listRaw`（admin 方法）、`SetWeight`？

```go
// 当前 kieai/accountPool.go 已有：
func NewAccountPool() *AccountPool
func (p *AccountPool) listRaw() []*kieAccount  // unexported, admin only
func (p *AccountPool) SetWeight(apiKey string, weight int) bool
func (p *AccountPool) Remove(apiKey string) bool
```

`listRaw` 是 unexported，subrouter 无法直接调用。需要将其改为 exported（`ListRaw`）或在 kieai 包内维护一个 admin alias。

**决策**：在 `kieai/accountPool.go` 末尾追加：
```go
// ListRaw returns raw pointers to internal accounts (for admin use only).
func (p *AccountPool) ListRaw() []*kieAccount {
    p.mu.RLock()
    defer p.mu.RUnlock()
    ret := make([]*kieAccount, len(p.accounts))
    copy(ret, p.accounts)
    return ret
}
```

- [ ] **Step 2：创建 subrouter channel.go**

```go
package subrouter

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "sync/atomic"
    "time"

    "goloop/internal/core"
    "goloop/internal/kieai"
    "goloop/internal/model"
)

// Config holds subrouter channel configuration.
type Config struct {
    BaseURL string
    Timeout time.Duration
}

// Channel implements core.Channel for a subrouter (OpenAI-compatible) upstream.
type Channel struct {
    name       string
    baseURL    string
    weight     atomic.Int64
    httpClient *http.Client
    pool       *kieai.AccountPool
    cfg        Config
}

// NewChannel creates a new subrouter channel.
func NewChannel(baseURL string, weight int, pool *kieai.AccountPool, cfg Config) *Channel {
    ch := &Channel{
        name:       "subrouter",
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: cfg.Timeout},
        pool:       pool,
        cfg:        cfg,
    }
    ch.weight.Store(int64(weight))
    return ch
}

func (ch *Channel) Name() string    { return ch.name }
func (ch *Channel) Weight() int    { return int(ch.weight.Load()) }
func (ch *Channel) SetChannelWeight(weight int) { ch.weight.Store(int64(weight)) }
func (ch *Channel) IsAvailable() bool { return ch.pool != nil && len(ch.pool.List()) > 0 }
func (ch *Channel) HealthScore() float64 {
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

func (ch *Channel) Generate(ctx context.Context, req *model.GoogleRequest, modelName string) (*model.GoogleResponse, error) {
    return nil, fmt.Errorf("subrouter: Generate not supported, use SubmitTask")
}

// OpenAI 请求/响应结构（与 Google 格式互转）
type ChatRequest struct {
    Model    string        `json:"model"`
    Messages []ChatMessage `json:"messages"`
    Stream   bool          `json:"stream,omitempty"`
}

type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type ChatResponse struct {
    ID      string   `json:"id"`
    Object  string   `json:"object"`
    Created int64    `json:"created"`
    Model   string   `json:"model"`
    Choices []Choice `json:"choices"`
    Usage   Usage    `json:"usage"`
}

type Choice struct {
    Index        int         `json:"index"`
    Delta        ChatMessage `json:"delta,omitempty"`        // 流式
    Message      ChatMessage `json:"message,omitempty"`       // 非流式
    FinishReason string      `json:"finish_reason"`
}

type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

// SubmitTask 将 Google 请求转为 OpenAI 格式，调用上游同步接口，缓存响应。
// 返回 (假taskID, apiKey, error)。
func (ch *Channel) SubmitTask(ctx context.Context, req *model.GoogleRequest, modelName string) (string, string, error) {
    acc, err := ch.pool.Select()
    if err != nil {
        return "", "", fmt.Errorf("subrouter: no account available: %w", err)
    }
    acc.IncUsage()

    openAireq := ch.googleToOpenAI(req, modelName)
    body, err := json.Marshal(openAireq)
    if err != nil {
        return "", "", fmt.Errorf("subrouter: marshal: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
        ch.baseURL+"/v1/chat/completions", bytes.NewReader(body))
    if err != nil {
        return "", "", err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())

    resp, err := ch.httpClient.Do(httpReq)
    if err != nil {
        return "", "", err
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
    if err != nil {
        return "", "", err
    }
    if resp.StatusCode != http.StatusOK {
        return "", "", fmt.Errorf("subrouter: HTTP %d: %s", resp.StatusCode, string(data))
    }

    var chatResp ChatResponse
    if err := json.Unmarshal(data, &chatResp); err != nil {
        return "", "", fmt.Errorf("subrouter: unmarshal: %w", err)
    }

    // 缓存响应，taskID 用 chatResp.ID
    ch.cacheResponse(chatResp.ID, &chatResp)

    return chatResp.ID, acc.APIKey(), nil
}

// PollTask 从缓存中取响应，转为 Google 格式返回。
func (ch *Channel) PollTask(ctx context.Context, apiKey, taskID string) (*model.GoogleResponse, error) {
    cached := ch.getCachedResponse(taskID)
    if cached == nil {
        return nil, fmt.Errorf("subrouter: no cached response for task %q", taskID)
    }
    return ch.openAIToGoogle(cached)
}

// SubmitTaskStreaming 流式调用上游，透传 SSE（逐块转为 Google SSE）。
func (ch *Channel) SubmitTaskStreaming(ctx context.Context, req *model.GoogleRequest, modelName string) (<-chan *model.GoogleResponse, string, error) {
    acc, err := ch.pool.Select()
    if err != nil {
        return nil, "", fmt.Errorf("subrouter: no account available: %w", err)
    }
    acc.IncUsage()

    openAireq := ch.googleToOpenAI(req, modelName)
    openAireq.Stream = true
    body, err := json.Marshal(openAireq)
    if err != nil {
        return nil, "", fmt.Errorf("subrouter: marshal: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
        ch.baseURL+"/v1/chat/completions", bytes.NewReader(body))
    if err != nil {
        return nil, "", err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+acc.APIKey())
    httpReq.Header.Set("Accept", "text/event-stream")

    resp, err := ch.httpClient.Do(httpReq)
    if err != nil {
        return nil, "", err
    }

    // 启动后台 goroutine 逐块读 SSE 并转换
    outCh := make(chan *model.GoogleResponse, 100)
    go ch.streamSSE(resp, outCh)
    return outCh, acc.APIKey(), nil
}

// --- 内部方法 ---

// responseCache 缓存 subrouter 的同步响应，供 PollTask 使用。
type responseCache struct {
    resp  *ChatResponse
    valid bool
}

var dummy struct{}

func (ch *Channel) googleToOpenAI(req *model.GoogleRequest, modelName string) *ChatRequest {
    msgs := []ChatMessage{}
    for _, content := range req.Contents {
        for _, part := range content.Parts {
            if part.Text != "" {
                msgs = append(msgs, ChatMessage{Role: "user", Content: part.Text})
            }
        }
    }
    return &ChatRequest{Model: modelName, Messages: msgs}
}

func (ch *Channel) openAIToGoogle(resp *ChatResponse) (*model.GoogleResponse, error) {
    if len(resp.Choices) == 0 {
        return &model.GoogleResponse{}, nil
    }
    choice := resp.Choices[0]
    parts := []model.Part{}
    if choice.Message.Content != "" {
        parts = append(parts, model.Part{Text: choice.Message.Content})
    }
    return &model.GoogleResponse{
        Candidates: []model.Candidate{{
            Content: model.Content{Parts: parts},
            FinishReason: choice.FinishReason,
        }},
    }, nil
}

// streamSSE 逐块读 OpenAI SSE，转为 Google SSE 写到 outCh。
// 注意：OpenAI 流式 chunk 格式：
//   data: {"id":"...","choices":[{"delta":{"content":"..."}}]}
//   data: [DONE]
func (ch *Channel) streamSSE(resp *http.Response, outCh chan<- *model.GoogleResponse) {
    defer resp.Body.Close()
    defer close(outCh)

    dec := json.NewDecoder(resp.Body)
    for dec.More() {
        var wrapper struct {
            Choices []struct {
                Delta struct {
                    Content string `json:"content"`
                } `json:"delta"`
                FinishReason string `json:"finish_reason"`
            } `json:"choices"`
        }
        if err := dec.Decode(&wrapper); err != nil {
            return
        }
        if len(wrapper.Choices) == 0 {
            continue
        }
        delta := wrapper.Choices[0].Delta.Content
        finish := wrapper.Choices[0].FinishReason
        if delta == "" && finish == "" {
            continue
        }
        parts := []model.Part{}
        if delta != "" {
            parts = append(parts, model.Part{Text: delta})
        }
        outCh <- &model.GoogleResponse{
            Candidates: []model.Candidate{{
                Content: model.Content{Parts: parts},
                FinishReason: finish,
            }},
        }
    }
}

// cacheResponse 简单内存缓存（taskID → ChatResponse）。
// subrouter 同步调用完成后缓存，PollTask 按 taskID 取用。
func (ch *Channel) cacheResponse(taskID string, resp *ChatResponse) {
    // TODO: 实现合理的缓存（可按需用 sync.Map 或固定大小 LRU）
}

func (ch *Channel) getCachedResponse(taskID string) *ChatResponse {
    // TODO: 从缓存取
    return nil
}
```

- [ ] **Step 3：构建验证**

运行：`go build ./internal/channels/subrouter/`
预期：PASS（需补全 import 和修复类型错误）

- [ ] **Step 4：提交**

---

### Task 2：添加账号操作方法

**文件：**
- 修改：`kieai/accountPool.go`（新增 `ListRaw`）
- 修改：`internal/channels/subrouter/channel.go`

- [ ] 在 `kieai/accountPool.go` 末尾追加：

```go
// ListRaw returns raw pointers to internal accounts (for admin use only).
func (p *AccountPool) ListRaw() []*kieAccount {
    p.mu.RLock()
    defer p.mu.RUnlock()
    ret := make([]*kieAccount, len(p.accounts))
    copy(ret, p.accounts)
    return ret
}
```

- [ ] 在 `subrouter/channel.go` 追加 ListAccounts/ResetAccount/RetireAccount/ProbeAccount/SetWeight（与 kieai 完全相同）：

```go
func (ch *Channel) ListAccounts() []map[string]any { ... }
func (ch *Channel) ResetAccount(apiKey string) bool { ... }
func (ch *Channel) RetireAccount(apiKey string) bool { ... }
func (ch *Channel) ProbeAccount(apiKey string) bool { ... }
func (ch *Channel) SetWeight(apiKey string, weight int) bool { ... }
```

- [ ] **Step 3：构建验证**

运行：`go build ./...`
预期：PASS

- [ ] **Step 4：提交**

---

## Chunk 2：响应缓存实现

`getCachedResponse` 目前返回 nil。实现一个简单的内存缓存。

**文件：**
- 修改：`internal/channels/subrouter/channel.go`

- [ ] **Step 1：实现缓存**

```go
// responseCache 简单内存缓存。
type responseCache struct {
    mu  sync.RWMutex
    m   map[string]*ChatResponse
}

func newResponseCache() *responseCache {
    return &responseCache{m: make(map[string]*ChatResponse)}
}

func (c *responseCache) Set(id string, resp *ChatResponse) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.m[id] = resp
}

func (c *responseCache) Get(id string) *ChatResponse {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.m[id]
}

// Channel 结构体新增字段：
//   cache *responseCache
// NewChannel 中初始化：ch.cache = newResponseCache()

// cacheResponse 和 getCachedResponse 改为调用 ch.cache.Set/Get。
```

- [ ] **Step 2：构建验证**

运行：`go build ./...`
预期：PASS

- [ ] **Step 3：提交**

---

## Chunk 3：main.go 集成与模型映射

### Task 3：注册 subrouter 渠道到 main.go

**文件：**
- 修改：`cmd/server/main.go`

- [ ] 在 channel 启动循环中新增 case：

```go
case "subrouter":
    pool := subrouter.NewAccountPool()  // subrouter.NewAccountPool = kieai.NewAccountPool
    for _, acc := range chCfg.Accounts {
        pool.AddAccount(acc.APIKey, acc.Weight)
    }
    timeout := chCfg.Timeout
    if timeout == 0 {
        timeout = 60 * time.Second
    }
    subCh := subrouter.NewChannel(chCfg.BaseURL, chCfg.Weight, pool, subrouter.Config{
        BaseURL: chCfg.BaseURL,
        Timeout: timeout,
    })
    registry.Register(subCh)
```

- [ ] **Step 2：构建验证**

运行：`go build ./...`
预期：PASS

- [ ] **Step 3：提交**

---

### Task 4：添加 subrouter 模型映射

**文件：**
- 修改：`internal/config/config.go`
- 修改：`.env.example`

- [ ] 在 `config.go` 的 `ModelMapping` 中追加：

```go
// subrouter 渠道的模型 — 请求内部转 OpenAI 格式透传上游
"gemini-3.1-pro-preview": {
    Channel:    "subrouter",
    KieAIModel: "gemini-3.1-pro-preview", // subrouter 直接透传此 model 名
},
"gemini-3-pro-preview": {
    Channel:    "subrouter",
    KieAIModel: "gemini-3-pro-preview",
},
```

- [ ] 在 `.env.example` 末尾追加：

```env
# subrouter 渠道（OpenAI 兼容上游）
# CHANNEL_SUBROUTER_BASE_URL=https://subrouter.ai
# CHANNEL_SUBROUTER_WEIGHT=50
# CHANNEL_SUBROUTER_ACCOUNTS=sk-key1:100
# CHANNEL_SUBROUTER_TIMEOUT=60s
```

- [ ] **Step 2：构建验证**

运行：`go build ./...`
预期：PASS

- [ ] **Step 3：提交**

---

## 待明确问题

1. **SSE 格式转换**：`streamSSE` 中 OpenAI SSE `data: {...}` 逐块转为 `model.GoogleResponse` 后，由谁写回客户端？当前 `streamSSE` 写到 `outCh`，但 `GeminiHandler` 的 `handleGenerateContentStreaming` 期望 `model.StreamingResponse` 格式。需要确认 subrouter 流式响应的目标格式是 Google SSE（与 kieai 一致）还是直接透传 OpenAI SSE？

2. **探活模型**：`Probe` 当前用 `"gpt-4o-mini"` 是否需要改为可配置？
