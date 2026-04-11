# 多渠道 AI 网关技术规格文档

> **状态**: v0.2（新增健康恢复机制）
> **日期**: 2026-04-11
> **架构风格**: 微内核 + 插件化

---

## 1. 背景与目标

当前 `goloop` 服务是一个单渠道（KIE.AI）的 Google Gemini API 适配中间件。扩展目标：

- **自颁发 JWT**：服务自行签发 JWT 令牌，只有携带有效 JWT 的请求才能调用 API；同时支持透传 JWT 给下游服务
- **多渠道架构**：KIE.AI 是第一个渠道，未来可扩展 Gemini Direct、Replicate、Stability AI 等渠道
- **多账号轮转**：每个渠道支持多个 API Key，按权重随机轮转（Weighted Random）
- **可靠性路由**：按权重优先 + 渠道健康度 + 故障自动切换，保障用户体验稳定性
- **输入输出一致性**：对外 API 始终是 Google Gemini 格式，各渠道内部自行做格式转换

---

## 2. 系统架构

### 2.1 架构概览

```
                              ┌──────────────────────────────────────┐
                              │              客户端                   │
                              │   (Gemini SDK / curl / 应用)          │
                              └──────────────────┬───────────────────┘
                                                 │ HTTP + JWT
                                                 ▼
                              ┌──────────────────────────────────────┐
                              │           JWT 验证层                  │
                              │  · 验证 JWT 有效性                    │
                              │  · 提取 claims（用户 ID、API Key、渠道）│
                              │  · 支持透传 JWT 给下游                 │
                              └──────────────────┬───────────────────┘
                                                 │
                    ┌────────────────────────────┼────────────────────────────┐
                    │                     核心层 (Core)                      │
                    │  ┌──────────────┐  ┌────────────┐  ┌────────────────┐  │
                    │  │ PluginRegistry│  │   Router   │  │ HealthTracker  │  │
                    │  │  插件注册表   │  │路由(权重+健康)│  │  健康度追踪    │  │
                    │  └──────────────┘  └────────────┘  └────────────────┘  │
                    │                                                       │
                    │  ┌──────────────┐  ┌─────────────────────────────┐    │
                    │  │ JWTIssuer   │  │      AccountPool 接口       │    │
                    │  │ JWT 签发/验证│  │  (每渠道独立的账号池)       │    │
                    │  └──────────────┘  └─────────────────────────────┘    │
                    └───────────────────────────────┬────────────────────────┘
                                                    │
              ┌─────────────────┬───────────────────┼───────────────────┬─────────────────┐
              │                 │                   │                   │                 │
              ▼                 ▼                   ▼                   ▼                 ▼
      ┌──────────────┐ ┌──────────────┐   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
      │  KIE.AI     │ │  Gemini      │   │  Replicate   │  │  Stability   │  │   Future     │
      │  Channel     │ │  Direct      │   │  Channel     │  │  Channel     │  │   Channel    │
      │  ──────────  │ │  Channel     │   │              │  │              │  │              │
      │  AccountPool │ │              │   │              │  │              │  │              │
      │  (权重随机)   │ │              │   │              │  │              │  │              │
      └──────────────┘ └──────────────┘   └──────────────┘  └──────────────┘  └──────────────┘
```

### 2.2 核心接口（Core Interface）

#### Channel 接口

每个 AI provider 插件必须实现 `core.Channel` 接口：

```go
type Channel interface {
    Name() string                          // 唯一标识，如 "kieai"
    Generate(ctx, apiKey, req, model)      // 同步生成（某些 provider 不支持则返回 error）
    SubmitTask(ctx, apiKey, req, model)    // 提交异步任务，返回 taskID
    PollTask(ctx, apiKey, taskID)          // 轮询任务结果
    HealthScore() float64                   // 健康度 0.0 ~ 1.0
    IsAvailable() bool                      // 是否可接收新请求
}
```

#### Account 接口

每个渠道内的 API Key 账号必须实现 `core.Account` 接口：

```go
type Account interface {
    APIKey() string                         // 原始 API Key
    Weight() int                           // 选权重，高权重被选概率大
    UsageCount() int                       // 已分配请求数
    HealthScore() float64                  // 健康度
    IsHealthy() bool                       // 是否可用
    IncUsage()                             // 增加使用计数
    RecordFailure()                        // 记录一次失败
    RecordSuccess()                         // 记录一次成功（降低失败计数）
}
```

#### AccountPool 接口

```go
type AccountPool interface {
    Select() (Account, error)              // 权重随机选一个可用账号
    Return(account Account, success bool)   // 用完后归还，标记成功/失败
}
```

### 2.3 核心组件

| 组件 | 职责 |
|------|------|
| `PluginRegistry` | 管理所有已注册的 Channel 插件，支持按名字查询 |
| `Router` | 依据权重随机 + 健康度选择最优 Channel；记录每次调用的成功/失败/延迟 |
| `HealthTracker` | 按 Channel 追踪连续失败次数、成功率、延迟，输出健康度分数 |
| `JWTIssuer` | 签发和验证 JWT 令牌，内嵌用户 API Key 和渠道限制 |
| `JWTMiddleware` | HTTP 中间件，验证请求头中的 JWT 并注入 claims 到 context |

---

## 3. 请求流程

### 3.1 完整请求时序

```
客户端                    网关                      Core                     KIE.AI Channel
  │                        │                        │                          │
  │ POST /v1beta/models/...│                        │                          │
  │ Authorization: Bearer JWT│                      │                          │
  │───────────────────────>│                        │                          │
  │                        │ JWT 验证                │                          │
  │                        │────────────────────────>│                          │
  │                        │                        │ 提取 claims（api_key）     │
  │                        │<────────────────────────│                          │
  │                        │                        │                          │
  │                        │ Router.Route()         │                          │
  │                        │────────────────────────>│                          │
  │                        │                        │ 权重随机 + 健康度筛选      │
  │                        │<────────────────────────│ 返回 KIE.AI Channel       │
  │                        │                        │                          │
  │                        │ channel.SubmitTask()   │                          │
  │                        │───────────────────────────────────────────────────>│
  │                        │                        │     taskID: "abc123"      │
  │                        │<──────────────────────────────────────────────────│
  │                        │                        │                          │
  │                        │ channel.PollTask()     │                          │
  │                        │───────────────────────────────────────────────────>│
  │                        │                        │   GET /recordInfo?taskId  │
  │                        │                        │   { state: "generating" } │
  │                        │<──────────────────────────────────────────────────│
  │                        │                        │  (等待中，指数退避...)      │
  │                        │                        │                          │
  │                        │ channel.PollTask()     │                          │
  │                        │───────────────────────────────────────────────────>│
  │                        │                        │   { state: "success",    │
  │                        │                        │     resultUrls: [...] }  │
  │                        │<──────────────────────────────────────────────────│
  │                        │                        │                          │
  │                        │ Router.RecordResult(success)                      │
  │                        │────────────────────────>│ (更新健康度)              │
  │                        │                        │                          │
  │ 200 OK (Gemini 格式)   │                        │                          │
  │<───────────────────────│                        │                          │
```

### 3.2 故障切换时序

```
当 KIE.AI Channel 的 HealthScore < 0.5 时：

Router.Route()
    │
    ├── KIE.AI Channel: score = 0.2 (不健康) → 排除
    │
    └── Gemini Direct Channel: score = 0.9 → 选中
            │
            └── 路由到 Gemini Direct Channel
                （若其他渠道也不可用，返回 503 Service Unavailable）
```

---

## 4. 各渠道设计

### 4.1 KIE.AI Channel

**职责**：封装 KIE.AI API，负责请求转换、任务提交、轮询、结果下载。

**账号池**（`kieai.AccountPool`）：
- 按权重随机选择账号：`weight * healthScore` 作为有效权重
- 连续失败 ≥5 次 → 标记 `healthy=false`，后续 `Select()` 自动排除
- 成功后失败计数 -1，最低为 0

**请求转换**（KIE.AI → Google）：
- 输入：Google Gemini API 格式（`model.GoogleRequest`）
- 输出：KIE.AI API 格式（`model.KieAICreateTaskRequest`）
- 模型映射：按 `config.model_mapping` 中的 `kieai_model` 字段

**响应转换**（KIE.AI → Google）：
- 下载所有 `resultUrls` 图片（并发 goroutine）
- base64 编码后嵌入 `inlineData`
- 返回 `model.GoogleResponse`

### 4.2 Future Channel（扩展模板）

每个新渠道只需实现 `core.Channel` 接口：

```go
type FutureChannel struct {
    pool   *AccountPool    // 复用通用的 AccountPool
    client *http.Client
    // ... 其他 provider 特定字段
}

func (ch *FutureChannel) Name() string     { return "future-channel" }
func (ch *FutureChannel) SubmitTask(...)   { /* provider 特定逻辑 */ }
func (ch *FutureChannel) PollTask(...)     { /* provider 特定逻辑 */ }
func (ch *FutureChannel) HealthScore() float64 { /* 聚合账号健康度 */ }
func (ch *FutureChannel) IsAvailable() bool { /* 检查连接性 */ }
// Generate 如果 provider 支持同步调用则实现，否则返回 error
```

注册到 `PluginRegistry`：

```go
registry.Register(futureChannel)
```

---

## 5. JWT 设计

### 5.1 JWT 结构

```json
{
  "sub": "user-123",
  "api_key": "kieai-key-abc",
  "channel": "kieai",
  "quota": 1000,
  "exp": 1747123200
}
```

| 字段 | 含义 |
|------|------|
| `sub` | 用户唯一标识 |
| `api_key` | 该用户在该渠道的 API Key |
| `channel` | 允许使用的渠道（可选，不填则不限制） |
| `quota` | 剩余配额（可选） |
| `exp` | 过期时间 |

### 5.2 Token 签发 API

```
POST /admin/issue-token

Request:
{
  "subject": "user-123",
  "api_key": "kieai-key-abc",
  "channel": "kieai"
}

Response:
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### 5.3 JWT 透传

如果下游服务也需要 JWT（用于追踪），网关可以将原始 JWT 放入请求头转发：

```
X-Forward-JWT: <original-jwt>
```

下游服务可以验证该 JWT 来确认请求来源。

---

## 6. 配置结构

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

channels:
  kieai:
    type: kieai
    base_url: https://api.kie.ai
    timeout: 120s
    weight: 100                    # 渠道权重，影响路由概率
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

  gemini-direct:
    type: gemini-direct
    base_url: https://generativelanguage.googleapis.com
    weight: 50
    accounts:
      - api_key: "${GEMINI_KEY}"
        weight: 100

model_mapping:
  gemini-3.1-flash-image-preview:
    channel: kieai
    kieai_model: nano-banana-2
    aspect_ratio: "1:1"
    resolution: "1K"
    output_format: png
```

---

## 7. 目录结构

```
goloop/
├── cmd/server/main.go                    # 服务入口，接线所有组件
├── internal/
│   ├── core/                            # 核心抽象（插件无关）
│   │   ├── channel.go                   # Channel 接口（含 Probe()）
│   │   ├── account.go                   # Account + AccountPool 接口
│   │   ├── pluginRegistry.go            # 插件注册表
│   │   ├── router.go                    # 权重随机 + 健康路由
│   │   ├── health.go                    # 健康度追踪器
│   │   ├── healthReaper.go              # 后台健康度探测 goroutine
│   │   └── jwt.go                       # JWT 签发/验证/中间件
│   ├── channels/                        # 各渠道插件
│   │   └── kieai/
│   │       ├── channel.go              # KIE.AI Channel 实现（含 Probe()）
│   │       ├── accountPool.go           # KIE.AI 账号池
│   │       ├── requestTransformer.go    # 请求格式转换
│   │       └── responseTransformer.go   # 响应格式转换
│   ├── handler/
│   │   ├── gemini_handler.go           # HTTP 处理器（重构后）
│   │   └── admin_handler.go             # Admin API（热插拔账号、手动探测）
│   ├── config/
│   │   └── config.go                    # 配置加载
│   └── storage/
│       └── image_storage.go             # 图片存储
└── config/config.yaml                   # 配置文件
```

---

## 8. API 端点

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `POST` | `/v1beta/models/{model}:generateContent` | JWT | 主接口，生成图片 |
| `GET` | `/v1beta/models` | 无 | 列出可用模型 |
| `GET` | `/health` | 无 | 健康检查 |
| `POST` | `/admin/issue-token` | 无（内部） | 签发 JWT |
| `GET` | `/images/{file}` | 无 | 访问已生成图片 |
| `GET` | `/admin/stats` | 无（内部） | 各渠道/账号统计 |
| `GET` | `/admin/channel/{channel}/accounts` | 无（内部） | 列出账号及状态 |
| `POST` | `/admin/channel/{channel}/accounts` | 无（内部） | 热添加账号 |
| `POST` | `/admin/channel/{channel}/accounts/{id}/reset` | 无（内部） | 重置账号健康状态 |
| `POST` | `/admin/channel/{channel}/accounts/{id}/retire` | 无（内部） | 永久下线账号 |
| `POST` | `/admin/channel/{channel}/accounts/{id}/probe` | 无（内部） | 手动触发探测 |

---

## 9. 错误处理

| 场景 | HTTP 状态码 | 错误状态 |
|------|------------|---------|
| JWT 缺失或无效 | 401 | `UNAUTHENTICATED` |
| 配额耗尽 | 402 | `QUOTA_EXCEEDED` |
| 请求格式错误 | 400 | `INVALID_ARGUMENT` |
| 渠道均不可用 | 503 | `SERVICE_UNAVAILABLE` |
| KIE.AI 计费失败 | 402 | `RESOURCE_EXHAUSTED` |
| 内部错误 | 500 | `INTERNAL` |

---

## 10. 可扩展性

### 添加新渠道

1. 在 `internal/channels/` 下创建新目录（如 `replicate/`）
2. 实现 `core.Channel` 接口（`SubmitTask`/`PollTask`/`HealthScore`）
3. 注册到 `PluginRegistry`：

```go
// cmd/server/main.go
replicateCh := replicate.NewChannel(...)
registry.Register(replicateCh)
```

无需修改 Handler、Router 或其他已有代码。

### 添加新账号

在 `config.yaml` 的 `channels.<name>.accounts` 下追加新的账号条目和权重，重启服务即可生效。

---

## 11. 健康度追踪与自动恢复机制

### 11.1 问题背景

账号被标记 `unhealthy`（连续失败 ≥5 次）后，若无任何外部干预，将永远无法恢复。这在以下场景会造成问题：

- 所有账号均被标记 unhealthy → 系统返回 503
- 无新请求进入 → 无法通过自然流量触发恢复
- 外部服务已自愈（配额恢复、网络恢复）但网关不知情

因此需要**主动探测 + 自动恢复**机制，打破这个死锁。

### 11.2 渐进式自然恢复

当一个被标记为 unhealthy 的账号在后续请求中成功完成时：

```
RecordSuccess():
    consecutive_failures -= 2        // 而不是直接清零
    if consecutive_failures <= 0:
        consecutive_failures = 0
        healthy = true               // 自动恢复
```

**优点**：无需额外组件，完全依赖自然流量恢复
**缺点**：在所有账号 unhealthy 时存在死锁风险（需配合主动探测）

### 11.3 后台健康度探测（Probe）

`ChannelHealthReaper` 是一个后台 goroutine，定期对所有 unhealthy 账号发起探测请求：

```
┌──────────────────────────────────────────────────────────────┐
│              ChannelHealthReaper                              │
│                                                               │
│  每 30s（可配置）对所有 unhealthy 账号执行：                   │
│                                                               │
│  for account in pool.UnhealthyAccounts():                     │
│      ok := channel.Probe(account)                            │
│      if ok:                                                 │
│          account.RecordSuccess()   # 不累积失败计数            │
│          account.healthy = true                              │
│      else:                                                   │
│          pass                  # 仅保持 unhealthy，不加重惩罚  │
└──────────────────────────────────────────────────────────────┘
```

**Probe 实现**（Channel 接口扩展）：
```go
type Channel interface {
    // ... 现有方法 ...

    // Probe 发送轻量级探测请求，验证账号是否可用。
    // 返回 true 表示账号可恢复，false 表示仍不可用。
    // 不记录为失败，也不计入 consecutive_failures。
    Probe(account Account) bool
}
```

**KIE.AI Channel 的 Probe 实现**：
```go
func (ch *Channel) Probe(account Account) bool {
    // 发送一个极短超时的 GET /api/v1/user/info
    // 如果返回 200 → 账号可用（配额可能已恢复）
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, "GET",
        ch.baseURL+"/api/v1/user/info", nil)
    req.Header.Set("Authorization", "Bearer "+account.APIKey())

    resp, err := ch.httpClient.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    return resp.StatusCode == 200
}
```

**配置项**：
```yaml
health:
  probe_interval: 30s        # 探测间隔，默认 30s
  probe_timeout: 5s          # 探测超时，默认 5s
  recovery_threshold: 2        # 连续探测成功次数才恢复，默认 2
```

### 11.4 渐进式恢复阈值

账号从 unhealthy 恢复到 healthy 需要满足：

```
连续探测成功次数 ≥ recovery_threshold（默认 2）
```

这避免了一次探测成功就立即恢复（可能存在短暂的假阳性）。

### 11.5 Admin API — 手动干预

以下场景建议使用 Admin API：

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/admin/channel/{channel}/accounts/{accountId}/reset` | 重置指定账号的连续失败计数，立即恢复 |
| `POST` | `/admin/channel/{channel}/accounts/{accountId}/retire` | 永久下线指定账号，从 AccountPool 中移除 |
| `POST` | `/admin/channel/{channel}/accounts` | 热添加新账号（无需重启） |
| `GET` | `/admin/channel/{channel}/accounts` | 列出该渠道下所有账号状态 |
| `POST` | `/admin/channel/{channel}/accounts/{accountId}/probe` | 手动触发一次探测 |
| `GET` | `/admin/stats` | 查看各渠道、各账号的累计成功/失败/延迟统计 |

### 11.6 完整恢复流程图

```
账号标记 unhealthy（连续失败 ≥ 5）
         │
         ▼
┌─────────────────────┐
│ ChannelHealthReaper  │
│ 每 30s 执行探测     │
└────────┬────────────┘
         │
         ▼
    ┌─────────────┐
    │ Probe() 调用 │
    └──────┬──────┘
           │
     ┌─────┴─────┐
     │  探测成功   │   探测失败
     ▼           ▼
  consecutive  ──────────────────→  保持 unhealthy
  probe_ok++
  │
  ▼
consecutive probe_ok >= recovery_threshold?
     │
     ├─ 否 → 保持 unhealthy，等待下次探测
     │
     └─ 是 → consecutive_failures = 0
              healthy = true
              账号恢复可用
```

### 11.7 防止探测浪费配额

探测请求本身会消耗 API 调用配额。为了最小化浪费：

1. **仅对 unhealthy 账号探测**：healthy 账号不探测
2. **使用最低成本端点**：如 `/user/info` 而非生成接口
3. **使用独立探测 Token**：如果渠道支持服务级 Token（非用户 Token），优先使用
4. **探测失败不累积惩罚**：失败不增加 `consecutive_failures`，避免雪崩

### 11.8 组件更新

新增文件：
```
internal/core/healthReaper.go      — 后台探测 goroutine
internal/core/account.go          — 新增 Probe() 接口声明
internal/channels/kieai/channel.go — 新增 Probe() 实现
```

修改文件：
```
internal/core/health.go           — 新增 UnhealthyAccounts() 方法
internal/handler/admin.go         — Admin API（热插拔账号、手动探测）
cmd/server/main.go               — 启动时启动 HealthReaper
```

---

## 12. Admin 管理界面

### 12.1 设计目标

Admin UI 是一个独立的 Web 界面，供运维人员实时监控和干预网关行为，无需登录服务器或调用 API。

**核心功能：**
- 实时查看所有渠道和账号的健康状态
- 手动探测、恢复、下线账号
- 热添加新账号
- 查看流量统计和成功率
- 颁发 JWT Token

### 12.2 页面结构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  🛰 goloop Admin                                          [颁发 Token] [设置] │
├──────────────┬──────────────────────────────────────────────────────────────┤
│              │                                                              │
│  📊 概览      │   Channel Overview — 渠道总览卡片                            │
│              │   ┌────────────┐ ┌────────────┐ ┌────────────┐            │
│  📡 渠道管理   │   │  KIE.AI   │ │  Gemini    │ │  Future   │            │
│              │   │  ● 健康    │ │  ● 亚健康  │ │  ○ 离线   │            │
│  👤 账号池    │   │  3/3 账号 │ │  1/2 账号 │ │  0/0 账号 │            │
│              │   │  99.2% 成功率│ │  67.1%    │ │   —       │            │
│  🔧 工具      │   └────────────┘ └────────────┘ └────────────┘            │
│              │                                                              │
│  📝 颁发Token │   Account Pool — KIE.AI                                    │
│              │   ┌────────────────────────────────────────────────────┐   │
│  📈 统计      │   │  key-1  ●  healthy  │  w=50  │  1,234 req  │  99.8%  │   │
│              │   │  key-2  ●  healthy  │  w=30  │    876 req  │  98.1%  │   │
│  ⚙️ 设置      │   │  key-3  ○  unhealthy│  w=20  │    234 req  │  45.2%  │   │
│              │   └────────────────────────────────────────────────────┘   │
│              │   [探测] [重置] [下线]  [+ 添加账号]                         │
└──────────────┴──────────────────────────────────────────────────────────────┘
```

### 12.3 各页面功能

#### 12.3.1 概览（Dashboard）

- **渠道状态卡片**：每个渠道一个卡片，显示：健康状态指示灯、在线账号数/总账号数、7 天平均成功率、平均延迟
- **全局流量图**：折线图显示过去 24 小时请求量、成功率、p99 延迟
- **告警栏**：显示当前所有 unhealthy 账号和连续失败次数

#### 12.3.2 渠道管理（Channels）

- 列出所有已注册渠道
- 每个渠道显示：名称、类型、权重、当前健康分数、AccountPool 统计
- 支持展开查看该渠道下所有账号详情

#### 12.3.3 账号池（Account Pool）

**核心表格**：

| 账号ID | API Key（截断） | 权重 | 状态 | 累计请求 | 成功率 | 连续失败 | 最后使用 | 操作 |
|--------|----------------|------|------|---------|--------|---------|---------|------|
| acc-1 | kie_****abc | 50 | 🟢 healthy | 1,234 | 99.8% | 0 | 2min ago | [探测] [重置] [下线] |
| acc-2 | kie_****def | 30 | 🟢 healthy | 876 | 98.1% | 1 | 5min ago | [探测] [重置] [下线] |
| acc-3 | kie_****ghi | 20 | 🔴 unhealthy | 234 | 45.2% | 7 | 1hr ago | [探测] [重置] [下线] |

**状态指示**：
- 🟢 **Healthy**：正常可用
- 🟡 **Degraded**：健康度 < 0.7 但 > 0.5
- 🔴 **Unhealthy**：连续失败 ≥ 5，已被排除
- ⚫ **Offline**：AccountPool 中已下线

**操作按钮**：
- **探测**：立即向该账号发送一次 Probe 请求，更新状态
- **重置**：将连续失败计数归零，标记为 healthy
- **下线**：从 AccountPool 中永久移除（retire）
- **添加**（表格底部）：输入 API Key 和权重，热添加新账号

#### 12.3.4 工具（Tools）

- **颁发 Token**：输入 subject、api_key、channel，生成 JWT 并复制
- **批量探测**：对所有 unhealthy 账号立即触发一次 Probe
- **刷新状态**：强制刷新所有账号的当前状态

#### 12.3.5 统计（Stats）

- **按渠道**：请求量、成功率、平均延迟、p99 延迟
- **按账号**：请求量、成功率、失败原因分布
- **时间范围**：可切换 1h / 24h / 7d / 30d

#### 12.3.6 设置（Settings）

- **探测间隔**：调整 `probe_interval`（默认 30s）
- **恢复阈值**：调整 `recovery_threshold`（默认 2）
- **渠道权重**：调整各渠道的路由权重
- **导出配置**：将当前配置导出为 YAML

### 12.4 技术实现

**前端**：
- 纯 HTML + CSS + Vanilla JS（无框架依赖，直接打包进二进制）
- WebSocket 实时推送健康状态变化（可选）
- 所有数据通过 Admin API 获取

**后端**：
- 复用已有的 Admin API（`/admin/*`）
- 新增 `/admin/ui` 提供静态文件

**目录结构**：
```
internal/
├── admin/
│   ├── ui/
│   │   ├── index.html          # 单页应用入口
│   │   ├── styles.css
│   │   └── app.js             # 界面逻辑 + API 调用
│   └── admin_handler.go       # Admin API（已有）
```

### 12.5 Admin UI 界面 mockup

以下为设计稿（见可视化输出）：

**主界面布局**：
- 左侧导航栏（深色主题，图标 + 文字）
- 右侧内容区（深蓝灰底色，表格和卡片布局）
- 顶部状态栏（全局健康状态 + 快速操作）

**颜色语义**：
- 🟢 Healthy：`#4CAF50`（绿色）
- 🟡 Degraded：`#FF9800`（橙色）
- 🔴 Unhealthy：`#F44336`（红色）
- ⚫ Offline：`#9E9E9E`（灰色）
- 主题背景：`#0D1117`（深夜蓝）
- 卡片背景：`#161B22`
- 边框：`#30363D`
