# goloop — Gemini API 适配中间件

将 Google Gemini API 格式的图片生成请求透明转换为 KIE.AI 异步任务 API，支持轮询、多图返回、本地图片存储，响应完全兼容 Google API 格式。

---

## 目录

- [架构概览](#架构概览)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [运行测试](#运行测试)
- [启动服务](#启动服务)
- [API 使用](#api-使用)
- [Docker 部署](#docker-部署)
- [环境变量](#环境变量)

---

## 架构概览

```
客户端 (Gemini SDK / curl)
        │  POST /v1beta/models/{model}:generateContent
        ▼
┌─────────────────┐
│   HTTP Handler  │  提取 API Key，解析请求体，路由
└────────┬────────┘
         │
┌────────▼────────┐
│ RequestTransfor │  Google 格式 → KIE.AI 格式，处理 base64 图片上传
└────────┬────────┘
         │
┌────────▼────────┐
│   KIE.AI Client │  提交任务 POST /api/v1/jobs/createTask
└────────┬────────┘
         │
┌────────▼────────┐
│     Poller      │  指数退避轮询 GET /api/v1/jobs/recordInfo
│                 │  2s → 10s，最长 120s，连续失败 3 次中止
└────────┬────────┘
         │
┌────────▼────────┐
│ ResponseTransfo │  并发下载结果图片，base64 编码，返回 Google 格式
└─────────────────┘
```

**模型映射：**

| Gemini 模型 | KIE.AI 模型 | 类型 | 默认分辨率 |
|---|---|---|---|
| `gemini-3.1-flash-image-preview` | `nano-banana-2` | 文本生成图片 | 1K |
| `gemini-3-pro-image-preview` | `nano-banana-pro` | 文本生成图片 | 1K |
| `gemini-2.5-flash-image` | `google/nano-banana` | 文本生成图片 | 1K |
| `gemini-3.1-flash-image-edit` | `google/nano-banana-edit` | 图片编辑 | - |

---

## 快速开始

### 前置要求

- Go 1.23+
- KIE.AI API Key（[kie.ai](https://kie.ai)）

### 安装

```bash
git clone <repo-url>
cd goloop
go mod download
```

### 一键运行所有测试

```bash
go test ./... -timeout 60s
```

预期输出：

```
ok  goloop/internal/config       0.xxx s
ok  goloop/internal/handler      0.xxx s
ok  goloop/internal/kieai        0.xxx s
ok  goloop/internal/model        0.xxx s
ok  goloop/internal/storage      0.xxx s
ok  goloop/internal/transformer  0.xxx s
```

---

## 配置说明

配置文件位于 `config/config.yaml`，支持 `${ENV_VAR}` 环境变量展开。

```yaml
server:
  port: 8080
  read_timeout: 130s   # 需大于 KIE.AI 最长等待时间
  write_timeout: 130s

kieai:
  base_url: https://api.kie.ai
  timeout: 120s

poller:
  initial_interval: 2s    # 首次轮询等待
  max_interval: 10s       # 最大轮询间隔（指数退避上限）
  max_wait_time: 120s     # 总超时
  retry_attempts: 3       # 连续失败次数上限

storage:
  type: local
  local_path: /tmp/images                    # 图片保存目录
  base_url: http://localhost:8080/images     # 对外访问的 URL 前缀

model_mapping:
  gemini-3.1-flash-image-preview:
    kieai_model: nano-banana-2
    aspect_ratio: "1:1"
    resolution: "1K"
    output_format: png
  gemini-3.1-flash-image-edit:
    kieai_model: google/nano-banana-edit
    aspect_ratio: "1:1"
    resolution: "1K"
    output_format: png
```

> **提示：** `base_url` 支持环境变量，例如 `base_url: ${STORAGE_BASE_URL}`

---

## 运行测试

### 全量测试

```bash
go test ./... -timeout 60s
```

### 详细输出（含每个测试名）

```bash
go test ./... -v -timeout 60s
```

### 按包测试

```bash
# 配置加载
go test ./internal/config/... -v

# 存储层（含下载测试）
go test ./internal/storage/... -v

# KIE.AI 客户端 + 轮询器
go test ./internal/kieai/... -v -timeout 30s

# 请求/响应转换
go test ./internal/transformer/... -v

# Handler 单元测试 + 端到端集成测试
go test ./internal/handler/... -v -timeout 30s
```

### 竞态检测

```bash
go test ./... -race -timeout 60s
```

### 测试覆盖率

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out   # 浏览器查看报告
```

---

## 启动服务

### 本地直接运行

```bash
# 使用默认配置
go run ./cmd/server

# 指定配置文件
go run ./cmd/server --config config/config.yaml
```

### 编译后运行

```bash
go build -o goloop ./cmd/server
./goloop --config config/config.yaml
```

服务启动后日志示例：

```json
{"time":"...","level":"INFO","msg":"server starting","port":8080}
```

### 健康检查

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## API 使用

### 认证

支持两种方式传入 KIE.AI API Key（透传给 KIE.AI，无需额外配置）：

```bash
# 方式 1：x-goog-api-key 头（推荐，兼容 Gemini SDK）
-H "x-goog-api-key: YOUR_KIEAI_API_KEY"

# 方式 2：Bearer Token
-H "Authorization: Bearer YOUR_KIEAI_API_KEY"
```

### 文生图

```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [{"text": "a sunset over the ocean, photorealistic"}]
    }]
  }'
```

### 图生图（base64 输入）

```bash
IMAGE_B64=$(base64 -i input.png)

curl -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_KIEAI_API_KEY" \
  -d "{
    \"contents\": [{
      \"parts\": [
        {\"text\": \"make it look like a painting\"},
        {\"inlineData\": {\"mimeType\": \"image/png\", \"data\": \"$IMAGE_B64\"}}
      ]
    }]
  }"
```

### 图生图（URL 输入）

```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [
        {"text": "anime style"},
        {"fileData": {"mimeType": "image/png", "fileUri": "https://example.com/photo.png"}}
      ]
    }]
  }'
```

> **注意：** `fileUri` 必须以 `https://` 开头。

### 自定义分辨率/比例

```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_KIEAI_API_KEY" \
  -d '{
    "contents": [{"parts": [{"text": "a mountain landscape"}]}],
    "generationConfig": {
      "imageConfig": {
        "aspectRatio": "16:9",
        "imageSize": "2K",
        "outputFormat": "png"
      }
    }
  }'
```

### 图片编辑（Image Editing）

使用 `gemini-3.1-flash-image-edit` 模型编辑已有图片：

```bash
curl -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-edit:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: YOUR_KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [
        {"text": "Add a wizard hat to the cat and change the background to a magical forest"},
        {"inlineData": {"mimeType": "image/jpeg", "data": "<base64_encoded_image>"}}
      ]
    }],
    "generationConfig": {
      "imageConfig": {
        "aspectRatio": "1:1",
        "outputFormat": "png"
      }
    }
  }'
```

**图片编辑要求：**
- 必须提供至少 1 张图片（支持 base64 或 URL）
- 最多支持 10 张图片
- Prompt 最长 5000 字符
- 单张图片最大 10MB
- 支持的比例：`1:1`, `9:16`, `16:9`, `3:4`, `4:3`, `3:2`, `2:3`, `5:4`, `4:5`, `21:9`, `auto`

### 响应格式

```json
{
  "candidates": [{
    "content": {
      "parts": [
        {"text": "Generated 2 image(s) successfully."},
        {"inlineData": {"mimeType": "image/png", "data": "<base64>"}},
        {"inlineData": {"mimeType": "image/png", "data": "<base64>"}}
      ]
    },
    "finishReason": "STOP"
  }]
}
```

### 错误响应格式

```json
{
  "error": {
    "code": 401,
    "message": "API key not provided",
    "status": "UNAUTHENTICATED"
  }
}
```

| KIE.AI 状态码 | HTTP 状态码 | status 字段 |
|---|---|---|
| 401 | 401 | `UNAUTHENTICATED` |
| 402 / 429 | 429 | `RESOURCE_EXHAUSTED` |
| 422 | 400 | `INVALID_ARGUMENT` |
| 5xx / 其他 | 500 | `INTERNAL` |

### 访问已保存图片

生成的图片同时保存在本地，可通过 HTTP 直接访问：

```
http://localhost:8080/images/<filename>.png
```

---

## Docker 部署

### 构建镜像

```bash
docker build -t goloop:latest .
```

### 运行容器

```bash
docker run -d \
  --name goloop \
  -p 8080:8080 \
  -e KIEAI_BASE_URL=https://api.kie.ai \
  -v $(pwd)/config/config.yaml:/app/config/config.yaml \
  goloop:latest
```

### 使用 docker-compose

```yaml
# docker-compose.yml
services:
  goloop:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config/config.yaml:/app/config/config.yaml
      - /tmp/images:/tmp/images
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

```bash
docker-compose up -d
docker-compose logs -f
```

---

## 环境变量

`config.yaml` 中所有字符串值都支持 `${VAR}` 展开，常用配置：

| 变量 | 对应配置 | 示例 |
|---|---|---|
| `KIEAI_BASE_URL` | `kieai.base_url` | `https://api.kie.ai` |
| `STORAGE_BASE_URL` | `storage.base_url` | `https://cdn.example.com/images` |
| `STORAGE_LOCAL_PATH` | `storage.local_path` | `/data/images` |

在 `config.yaml` 中引用：

```yaml
kieai:
  base_url: ${KIEAI_BASE_URL}
storage:
  base_url: ${STORAGE_BASE_URL}
  local_path: ${STORAGE_LOCAL_PATH}
```

---

## 限制说明

| 项目 | 限制 |
|---|---|
| 单次请求体 | 10 MB |
| 单张 base64 图片（编码前）| 40 MB |
| 单张下载图片 | 30 MB |
| 单次请求最多图片数 | 14 张 |
| 提示词最大长度 | 20,000 字符 |
| 任务最长等待时间 | 120 秒 |
| `fileUri` 协议 | 仅 `https://` |
