---
name: Gemini API 适配中间件
overview: 构建一个 Go 实现的 HTTP 中间件服务,将 Google Gemini 官方 API 格式转换为 KIE.AI 异步任务 API,并处理轮询逻辑,最终返回兼容 Google 格式的响应。
todos:
  - id: setup-project
    content: 初始化 Go 项目结构和依赖
    status: pending
  - id: define-models
    content: 定义 Google 和 KIE.AI 的数据结构
    status: pending
  - id: implement-kieai-client
    content: 实现 KIE.AI API 客户端和轮询器
    status: pending
  - id: implement-transformers
    content: 实现请求和响应转换器
    status: pending
  - id: implement-handlers
    content: 实现 HTTP 处理器和路由
    status: pending
  - id: implement-storage
    content: 实现图片临时存储模块
    status: pending
  - id: add-config
    content: 添加配置管理和环境变量支持
    status: pending
  - id: add-logging
    content: 添加结构化日志和监控指标
    status: pending
  - id: write-tests
    content: 编写单元测试和集成测试
    status: pending
  - id: add-docker
    content: 添加 Dockerfile 和部署文档
    status: pending
isProject: false
---

# Gemini API 适配中间件实现方案

## 整体架构

采用"**兼容层 + 转换层 + 轮询层 + 归一化层**"的四层架构:

```
客户端 (Google API 格式请求)
  ↓
HTTP Handler (兼容 Google API 路由)
  ↓
Request Transformer (Google → KIE.AI)
  ↓
KIE.AI Client (异步任务提交)
  ↓
Task Poller (智能轮询机制)
  ↓
Response Transformer (KIE.AI → Google)
  ↓
客户端 (Google API 格式响应)
```

## 核心模块设计

### 1. API 兼容层 (`/internal/handler`)

**端点映射:**

- `POST /v1beta/models/{model}:generateContent` → KIE.AI 适配器
  - `model` 支持: `gemini-3.1-flash-image-preview`, `gemini-3-pro-image-preview`, `gemini-2.5-flash-image`

**模型映射关系:**


| Google 模型                        | KIE.AI 模型            |
| -------------------------------- | -------------------- |
| `gemini-3.1-flash-image-preview` | `nano-banana-2`      |
| `gemini-3-pro-image-preview`     | `nano-banana-pro`    |
| `gemini-2.5-flash-image`         | `google/nano-banana` |


**请求结构 (Google 格式):**

```json
{
  "contents": [{
    "parts": [
      {"text": "prompt text"},
      {"inlineData": {"mimeType": "image/png", "data": "base64..."}}
    ]
  }],
  "generationConfig": {
    "responseModalities": ["TEXT", "IMAGE"]
  }
}
```

**响应结构 (Google 格式):**

```json
{
  "candidates": [{
    "content": {
      "parts": [
        {"text": "描述文本"},
        {"inlineData": {"mimeType": "image/png", "data": "base64..."}}
      ]
    },
    "finishReason": "STOP"
  }]
}
```

### 2. 请求转换层 (`/internal/transformer`)

**转换逻辑:**

1. 从 `contents[].parts[]` 提取 prompt 文本
2. 从 `contents[].parts[]` 提取 inlineData 图片,转换为 URL
3. 从 URL 路径提取模型名并映射到 KIE.AI 模型
4. 生成 KIE.AI 请求结构

**KIE.AI 请求格式:**

```json
{
  "model": "nano-banana-2",
  "input": {
    "prompt": "extracted text",
    "image_input": ["url1", "url2"],
    "aspect_ratio": "1:1",
    "resolution": "1K",
    "output_format": "png"
  }
}
```

**图片处理策略:**

- 如果客户端提供 base64 图片,先上传到临时存储(如 S3/本地)获取 URL
- 如果客户端已提供 URL,直接使用

### 3. KIE.AI 客户端层 (`/internal/kieai`)

**职责:**

- 调用 `POST https://api.kie.ai/api/v1/jobs/createTask` 创建任务
- 轮询 `GET https://api.kie.ai/api/v1/jobs/recordInfo?taskId=xxx` 查询状态
- 处理错误码映射

**任务状态机:**

```
waiting/queuing/generating → 继续轮询
success → 下载图片并返回
fail → 转换为 Google 错误格式
```

**轮询策略 (指数退避):**

- 初始间隔: 2 秒
- 最大间隔: 10 秒
- 超时时间: 120 秒 (可配置)
- 重试策略: 轮询失败重试 3 次

### 4. 响应归一化层 (`/internal/normalizer`)

**成功响应转换:**

1. 从 `resultJson.resultUrls[0]` 下载图片
2. 将图片转换为 base64
3. 构造 Google 格式的 `candidates[0].content.parts[]`

**错误响应转换:**


| KIE.AI 错误码 | Google 错误码         | 错误信息                 |
| ---------- | ------------------ | -------------------- |
| 401        | UNAUTHENTICATED    | Invalid API key      |
| 402        | RESOURCE_EXHAUSTED | Insufficient credits |
| 422        | INVALID_ARGUMENT   | Validation failed    |
| 500/501    | INTERNAL           | Generation failed    |
| 429        | RESOURCE_EXHAUSTED | Rate limit exceeded  |


## 项目代码结构

```
goloop/
├── cmd/
│   └── server/
│       └── main.go                    # 服务入口
├── internal/
│   ├── handler/
│   │   └── gemini_handler.go         # HTTP 处理器
│   ├── transformer/
│   │   ├── request_transformer.go    # Google → KIE.AI
│   │   └── response_transformer.go   # KIE.AI → Google
│   ├── kieai/
│   │   ├── client.go                 # KIE.AI API 客户端
│   │   └── poller.go                 # 任务轮询器
│   ├── normalizer/
│   │   ├── normalizer.go             # 响应归一化
│   │   └── validator.go              # Schema 校验
│   ├── storage/
│   │   └── image_storage.go          # 图片临时存储
│   └── model/
│       ├── google.go                  # Google API 结构体
│       └── kieai.go                   # KIE.AI API 结构体
├── config/
│   └── config.yaml                    # 配置文件
├── go.mod
└── README.md
```

## 配置设计

```yaml
server:
  port: 8080
  read_timeout: 130s  # 比轮询超时长
  write_timeout: 130s

kieai:
  base_url: https://api.kie.ai
  api_key: ${KIEAI_API_KEY}
  timeout: 120s
  
poller:
  initial_interval: 2s
  max_interval: 10s
  max_wait_time: 120s
  retry_attempts: 3

storage:
  type: local  # local | s3
  local_path: /tmp/images
  base_url: http://localhost:8080/images

model_mapping:
  gemini-3.1-flash-image-preview: nano-banana-2
  gemini-3-pro-image-preview: nano-banana-pro
  gemini-2.5-flash-image: google/nano-banana
```

## 关键实现细节

### 并发处理

- 每个客户端请求独立处理,互不阻塞
- 使用 `context.Context` 传递超时和取消信号
- Goroutine 安全的客户端连接池

### 错误处理

- 分层错误包装,便于排查
- 统一错误日志格式
- 客户端友好的错误信息

### 可观测性

**日志字段:**

- `request_id`: 唯一请求标识
- `google_model`: 客户端请求的模型
- `kieai_model`: 映射后的 KIE.AI 模型
- `task_id`: KIE.AI 任务 ID
- `task_state`: 任务状态
- `poll_count`: 轮询次数
- `total_duration`: 总耗时

**指标监控:**

- 请求成功率
- 平均响应时间 (P50/P95/P99)
- KIE.AI 任务状态分布
- 轮询次数分布
- 图片转换耗时

### 性能优化

1. **图片处理优化**
  - 使用流式下载避免大图片占用内存
  - 并发下载多个结果 URL
2. **HTTP 客户端复用**
  - 使用单例 HTTP 客户端
  - 启用连接池和 Keep-Alive
3. **响应缓存 (可选)**
  - 对相同 prompt 的结果缓存一段时间
  - 使用 Redis 或内存缓存

## API 使用示例

### 文本生成图片 (curl)

```bash
curl -X POST \
  'http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent' \
  -H 'Content-Type: application/json' \
  -H 'x-goog-api-key: YOUR_KIEAI_API_KEY' \
  -d '{
    "contents": [{
      "parts": [
        {"text": "A beautiful sunset over mountains"}
      ]
    }]
  }'
```

### 图片编辑 (Python SDK 风格)

```python
# 虽然是中间件,但接口完全兼容 Google SDK
from google import genai

client = genai.Client(api_endpoint="http://localhost:8080")
response = client.models.generate_content(
    model="gemini-3.1-flash-image-preview",
    contents=["Edit this image to add a rainbow"]
)
```

## 安全考虑

1. **API Key 验证**
  - 从 `x-goog-api-key` 或 `Authorization: Bearer` 提取
  - 传递给 KIE.AI 进行验证
2. **输入验证**
  - Prompt 长度限制 (20000 字符)
  - 图片大小限制 (30MB)
  - 图片数量限制 (14 张)
3. **Rate Limiting**
  - 基于 IP 或 API Key 的限流
  - 使用 `golang.org/x/time/rate`

## 测试策略

1. **单元测试**: 每个 transformer 和 normalizer 独立测试
2. **集成测试**: Mock KIE.AI API 响应
3. **端到端测试**: 实际调用 KIE.AI API
4. **压力测试**: 模拟高并发场景

## 部署方案

**Docker 部署:**

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o server cmd/server/main.go

FROM alpine:latest
COPY --from=builder /app/server /server
COPY config/config.yaml /config.yaml
CMD ["/server", "--config", "/config.yaml"]
```

**Kubernetes 部署:**

- Deployment: 3 副本保证高可用
- Service: ClusterIP 或 LoadBalancer
- ConfigMap: 配置文件
- Secret: API Key

## 演进路线

**第一阶段 (MVP):**

- 实现基础转换逻辑
- 支持 text-to-image
- 轮询机制
- 本地图片存储

**第二阶段:**

- 支持 image-to-image 编辑
- S3/OSS 图片存储
- 完整错误处理
- 日志和监控

**第三阶段:**

- 响应缓存
- WebSocket 推送 (替代轮询)
- 多区域部署
- 管理后台

