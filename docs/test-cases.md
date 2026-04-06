# goloop E2E 测试用例文档

> **前置条件**
> - 服务已在本地启动：`go run ./cmd/server`（默认监听 `:8080`）
> - 持有有效的 KIE.AI API Key
> - 将 Key 写入 `.env.test`（已被 `.gitignore` 保护，不会提交）：
>
> ```bash
> cp .env.test.example .env.test
> # 编辑 .env.test，填入 KIEAI_API_KEY=your-key
> ```
>
> 所有 `curl` 示例中将 `$KIEAI_API_KEY` 替换为实际 Key，或直接 `source .env.test` 后执行。

---

## TC-01 文生图（基础流程）

**目的：** 验证从文本 Prompt 到返回 base64 图片的完整链路。

**请求：**

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [{"text": "a red apple on a white table, photorealistic"}]
    }]
  }'
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `200 OK` |
| `candidates` 数组 | 长度 ≥ 1 |
| `candidates[0].content.parts` | 包含 1 条 `text` + ≥ 1 条 `inlineData` |
| `inlineData.mimeType` | `"image/png"` |
| `inlineData.data` | 非空、合法 base64 字符串 |
| `finishReason` | `"STOP"` |

**响应结构示例：**

```json
{
  "candidates": [{
    "content": {
      "parts": [
        {"text": "Generated 1 image(s) successfully."},
        {"inlineData": {"mimeType": "image/png", "data": "<base64>"}}
      ]
    },
    "finishReason": "STOP"
  }]
}
```

---

## TC-02 自定义宽高比与分辨率（imageConfig 覆盖）

**目的：** 验证 `generationConfig.imageConfig` 能覆盖模型默认的 `aspect_ratio` 和 `imageSize`。

**请求：**

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [{"text": "wide panoramic forest at dusk"}]
    }],
    "generationConfig": {
      "imageConfig": {
        "aspectRatio": "16:9",
        "imageSize": "2K",
        "outputFormat": "png"
      }
    }
  }'
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `200 OK` |
| `inlineData` 图片 | ≥ 1 张，内容为宽图（16:9 比例） |

**验证要点：** 该模型默认 `aspect_ratio: 1:1`，此用例确认客户端参数能成功覆盖。

---

## TC-03 图生图（base64 内联图片输入）

**目的：** 验证 `inlineData` 类型的图片输入能被正确上传并传给 KIE.AI。

**准备测试图片：**

```bash
# 用任意小图片，转换为 base64
IMAGE_B64=$(base64 -i /path/to/input.png | tr -d '\n')
```

**请求：**

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d "{
    \"contents\": [{
      \"parts\": [
        {\"text\": \"make it look like an oil painting\"},
        {\"inlineData\": {\"mimeType\": \"image/png\", \"data\": \"$IMAGE_B64\"}}
      ]
    }]
  }"
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `200 OK` |
| 图片结果 | ≥ 1 张 `inlineData` |

**验证要点：** 中间件将 base64 解码 → 保存到 `/tmp/images/` → 构造 HTTP URL 传给 KIE.AI。可在 `/tmp/images/` 中确认临时文件存在。

---

## TC-04 图生图（URL 输入 fileData）

**目的：** 验证 `fileData.fileUri` 能作为图片输入直接传给 KIE.AI。

**请求：**

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [
        {"text": "anime style version"},
        {"fileData": {"mimeType": "image/png", "fileUri": "https://upload.wikimedia.org/wikipedia/commons/4/47/PNG_transparency_demonstration_1.png"}}
      ]
    }]
  }'
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `200 OK` |
| 图片结果 | ≥ 1 张 `inlineData` |

**边界验证（fileUri 使用 http:// 协议，应被拒绝）：**

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"fileData":{"fileUri":"http://example.com/img.png"}}]}]}'
```

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `400 Bad Request` |
| `error.status` | `"INVALID_ARGUMENT"` |

---

## TC-05 多模型切换

**目的：** 验证三种模型都能路由到各自的 KIE.AI 模型。

```bash
# Flash 模型
curl -s -o /dev/null -w "%{http_code}" -X POST \
  http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"test"}]}]}'

# Pro 模型
curl -s -o /dev/null -w "%{http_code}" -X POST \
  http://localhost:8080/v1beta/models/gemini-3-pro-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"test"}]}]}'

# Flash-Image 模型
curl -s -o /dev/null -w "%{http_code}" -X POST \
  http://localhost:8080/v1beta/models/gemini-2.5-flash-image:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"test"}]}]}'
```

**期望结果：** 三个请求均返回 `200`。

---

## TC-06 认证失败（无 API Key）

**目的：** 验证缺少 API Key 时返回标准 Google 格式的 401 错误。

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"test"}]}]}'
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `401 Unauthorized` |
| `error.code` | `401` |
| `error.status` | `"UNAUTHENTICATED"` |
| `error.message` | `"API key not provided"` |

```json
{
  "error": {
    "code": 401,
    "message": "API key not provided",
    "status": "UNAUTHENTICATED"
  }
}
```

---

## TC-07 认证失败（Key 通过 Bearer Token 传入）

**目的：** 验证 `Authorization: Bearer` 方式也能正常透传。

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"a sunset"}]}]}'
```

**期望结果：** `200 OK`，效果与 `x-goog-api-key` 方式相同。

---

## TC-08 未知模型（400 错误）

**目的：** 验证请求了未在 `model_mapping` 中配置的模型时返回 `400`。

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-unknown-xyz:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"test"}]}]}'
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `400 Bad Request` |
| `error.status` | `"INVALID_ARGUMENT"` |

---

## TC-09 健康检查

**目的：** 验证 `/health` 端点正常响应，可作为 Docker healthcheck 和监控探针。

```bash
curl -s http://localhost:8080/health
```

**期望结果：**

| 字段 | 期望值 |
|---|---|
| HTTP 状态码 | `200 OK` |
| 响应体 | `{"status":"ok"}` |
| 响应时间 | < 100ms |

---

## TC-10 多图返回验证

**目的：** 验证 KIE.AI 返回多张图片时，所有图片都出现在响应的 `parts[]` 中。

```bash
curl -s -X POST http://localhost:8080/v1beta/models/gemini-3.1-flash-image-preview:generateContent \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{
    "contents": [{
      "parts": [{"text": "four seasons: spring summer autumn winter, split view"}]
    }]
  }' | python3 -c "
import sys, json
resp = json.load(sys.stdin)
parts = resp['candidates'][0]['content']['parts']
images = [p for p in parts if 'inlineData' in p]
print(f'total parts: {len(parts)}, images: {len(images)}')
for i, img in enumerate(images):
    import base64
    size = len(base64.b64decode(img['inlineData']['data']))
    print(f'  image[{i}]: {img[\"inlineData\"][\"mimeType\"]}, {size} bytes')
"
```

**期望结果：** 打印出 ≥ 1 张图片信息，每张 size > 0。

---

## 快速回归脚本

将以上核心用例串联成一个 shell 脚本，批量验证：

```bash
#!/usr/bin/env bash
set -e

# 加载本地 Key（.env.test 不会被提交）
source .env.test

BASE="http://localhost:8080"
MODEL="gemini-3.1-flash-image-preview"
PASS=0; FAIL=0

check() {
  local name="$1" expect="$2" actual="$3"
  if [ "$actual" = "$expect" ]; then
    echo "  ✓ $name"
    ((PASS++))
  else
    echo "  ✗ $name — expected $expect, got $actual"
    ((FAIL++))
  fi
}

echo "=== TC-09 健康检查 ==="
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/health")
check "health 200" "200" "$code"

echo "=== TC-06 无 Key → 401 ==="
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1beta/models/$MODEL:generateContent" \
  -H "Content-Type: application/json" -d '{"contents":[{"parts":[{"text":"t"}]}]}')
check "no key 401" "401" "$code"

echo "=== TC-08 未知模型 → 400 ==="
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1beta/models/unknown-model-xyz:generateContent" \
  -H "Content-Type: application/json" -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"t"}]}]}')
check "unknown model 400" "400" "$code"

echo "=== TC-04 http:// fileUri → 400 ==="
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1beta/models/$MODEL:generateContent" \
  -H "Content-Type: application/json" -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"fileData":{"fileUri":"http://example.com/img.png"}}]}]}')
check "http fileUri 400" "400" "$code"

echo "=== TC-01 文生图 → 200 ==="
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1beta/models/$MODEL:generateContent" \
  -H "Content-Type: application/json" -H "x-goog-api-key: $KIEAI_API_KEY" \
  -d '{"contents":[{"parts":[{"text":"a red apple, photorealistic"}]}]}')
check "text to image 200" "200" "$code"

echo ""
echo "结果：$PASS 通过，$FAIL 失败"
[ "$FAIL" -eq 0 ]
```

保存为 `scripts/e2e_smoke.sh`，执行：

```bash
chmod +x scripts/e2e_smoke.sh
./scripts/e2e_smoke.sh
```

---

