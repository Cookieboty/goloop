# 🚀 启动指南

## 前置要求

1. **PostgreSQL** - 数据库
2. **Redis** - API Key 缓存（可选，但强烈推荐）
3. **Go 1.21+** - 后端运行时
4. **Node.js 18+** - 前端开发

---

## 快速启动

### 1. 配置环境变量

复制示例配置文件：

```bash
cp .env.example .env
```

编辑 `.env` 文件，必填项：

```env
# 管理员认证
JWT_SECRET=your-super-secret-jwt-key-here
ADMIN_PASSWORD=your-secure-admin-password

# 数据库（必填）
DATABASE_URL=postgresql://user:password@localhost:5432/goloop?sslmode=disable

# Redis（强烈推荐，用于 API Key 缓存）
REDIS_ENABLED=true
REDIS_URL=redis://localhost:6379/0
```

### 2. 初始化数据库

后端会自动运行数据库迁移（Auto Migration），首次启动时会创建所有必要的表：

- `channels` - 渠道配置
- `accounts` - 账号池
- `model_mappings` - 模型映射
- `api_keys` - API 密钥
- `usage_logs` - 使用日志

**无需手动执行 SQL 脚本！**

### 3. 启动后端

```bash
# 安装 Go 依赖
go mod download

# 编译并运行
go run cmd/server/main.go
```

后端默认监听 `:8080`，启动日志示例：

```
2024-04-24 10:00:00 INFO  Database connected
2024-04-24 10:00:00 INFO  Redis connected
2024-04-24 10:00:00 INFO  Loaded 3 channels from database
2024-04-24 10:00:00 INFO  Usage logger started
2024-04-24 10:00:00 INFO  Log cleaner scheduled (daily at 2 AM)
2024-04-24 10:00:00 INFO  Server listening on :8080
```

### 4. 启动前端（开发模式）

```bash
cd web
npm install
npm run dev
```

前端默认监听 `http://localhost:3000`

---

## 首次使用流程

### 1. 登录管理后台

访问 `http://localhost:3000/login`

- **用户名**: `admin`
- **密码**: `.env` 中配置的 `ADMIN_PASSWORD`

### 2. 创建渠道

进入 **渠道管理** → **创建渠道**

示例配置：

```yaml
名称: gemini-main
类型: gemini_original (Google 原生 Gemini API)
Base URL: https://generativelanguage.googleapis.com
权重: 100
超时: 60 秒

账号列表:
  - API Key: AIza...xxxxx
    权重: 100
  - API Key: AIza...yyyyy
    权重: 100
```

点击"保存"，渠道及账号会被事务性创建。

### 3. 颁发 API Key

进入 **API Key 管理** → **创建 API Key**

```yaml
名称: 测试密钥
渠道限制: 留空（不限制）
过期时间: 留空（永不过期）
```

点击"创建"，系统会生成一个类似 `goloop_abc123xyz...` 的密钥。

**⚠️ 重要：立即复制保存，关闭后将无法再次查看！**

### 4. 调用 API

使用颁发的 API Key 调用 Gemini 或 OpenAI 兼容的 API：

#### Gemini 原生 API 示例

```bash
curl -X POST http://localhost:8080/gemini/v1/models/gemini-pro:generateContent \
  -H "Authorization: Bearer goloop_abc123xyz..." \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{
      "parts": [{"text": "Hello, Gemini!"}]
    }]
  }'
```

#### OpenAI 兼容 API 示例

```bash
curl -X POST http://localhost:8080/openai/v1/chat/completions \
  -H "Authorization: Bearer goloop_abc123xyz..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## 渠道类型说明

| 类型 | 说明 | API 路径 | 适用场景 |
|------|------|----------|----------|
| `gemini_original` | Google 原生 Gemini API | `/gemini/*` | 标准 Gemini 调用 |
| `gemini_openai` | OpenAI 兼容的 Gemini API | `/openai/*` | 使用 OpenAI SDK 调用 Gemini |
| `gemini_callback` | 异步轮询式 Gemini（原 kieai） | `/gemini/*` | 长时间生成任务 |
| `openai_original` | OpenAI 原生 API | `/openai/*` | 标准 OpenAI 调用 |
| `openai_callback` | 异步轮询式 OpenAI | `/openai/*` | 长时间生成任务 |

---

## 配置热重载

修改渠道配置、账号或模型映射后，**无需重启服务**！

在管理后台点击 **"🔄 重载配置"** 按钮，后端会从数据库重新加载配置到内存。

---

## 生产部署

### 1. 构建前端

```bash
cd web
npm run build
npm start  # 或使用 PM2、Docker
```

### 2. 编译后端

```bash
go build -o goloop cmd/server/main.go
./goloop
```

### 3. 使用 Docker Compose（推荐）

```yaml
version: '3.8'
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: goloop
      POSTGRES_USER: goloop
      POSTGRES_PASSWORD: secure_password
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass your_redis_password

  goloop:
    build: .
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgresql://goloop:secure_password@postgres:5432/goloop?sslmode=disable
      REDIS_URL: redis://:your_redis_password@redis:6379/0
      JWT_SECRET: ${JWT_SECRET}
      ADMIN_PASSWORD: ${ADMIN_PASSWORD}
    depends_on:
      - postgres
      - redis

  web:
    build: ./web
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_API_BASE: http://goloop:8080
    depends_on:
      - goloop

volumes:
  postgres_data:
```

---

## 监控和日志

### 使用日志查询

管理后台 → **API Key 管理** → 选择 Key → **查看日志**

可查看：
- 请求时间
- 渠道名称
- 模型
- 成功/失败状态
- 延迟（毫秒）
- 请求 IP

### 日志清理

系统自动清理 30 天前的日志，每天凌晨 2 点执行。

---

## 故障排查

### 1. 数据库连接失败

检查 `DATABASE_URL` 格式：

```
postgresql://用户名:密码@主机:端口/数据库名?sslmode=disable
```

确认 PostgreSQL 服务运行中：

```bash
psql -h localhost -U goloop -d goloop
```

### 2. Redis 连接失败

如果不使用 Redis，设置 `REDIS_ENABLED=false`。

**⚠️ 警告：禁用 Redis 会导致 API Key 验证走数据库，高并发时可能成为瓶颈！**

### 3. API 调用返回 401

- 检查 API Key 是否正确
- 检查 Key 是否被禁用或过期
- 检查 Key 是否有渠道限制

### 4. 前端无法加载

- 检查后端是否启动
- 检查 JWT 是否配置
- 清除浏览器缓存和 LocalStorage

### 5. 配置重载后不生效

- 检查 `/admin/api/reload` 接口返回状态
- 重启服务作为最后手段

---

## 常见问题

**Q: 如何修改模型映射？**

A: 目前模型映射通过渠道创建时批量设置，后续可以通过 API 单独管理。

**Q: 可以设置账号级别的权重吗？**

A: 可以！在渠道的账号列表中，每个账号都有独立的权重设置。

**Q: 如何限制 API Key 只能访问特定渠道？**

A: 创建 API Key 时，在"渠道限制"字段填写渠道名称（例如 `gemini-main`）。

**Q: 日志存储会占用很多空间吗？**

A: 系统自动清理 30 天前的日志，可以根据需要调整 `LogCleaner` 的保留天数。

**Q: 支持多管理员吗？**

A: 当前版本使用单一的 `ADMIN_PASSWORD`，未来可扩展为多用户系统。

---

## 更多文档

- 后端实现详情：`IMPLEMENTATION_COMPLETE.md`
- 前端实现详情：`FRONTEND_COMPLETE.md`
- 迁移计划：`/uploads/渠道配置数据库迁移.plan`

---

**祝你使用愉快！** 🎉

如有问题，请查阅文档或提交 Issue。
