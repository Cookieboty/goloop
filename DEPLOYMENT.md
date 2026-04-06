# 部署指南

本文档介绍如何使用 Docker 和 GitHub Actions 部署 goloop 服务。

---

## Docker Compose 部署

### 快速启动

```bash
# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down

# 停止并删除数据卷
docker-compose down -v
```

### 配置说明

`docker-compose.yml` 已经配置好了以下功能：

- **端口映射**: 8080:8080
- **配置文件挂载**: `./config/config.yaml` 只读挂载到容器
- **持久化存储**: 使用 Docker volume 存储生成的图片
- **健康检查**: 每 30 秒检查一次服务状态
- **自动重启**: 容器异常退出时自动重启
- **日志轮转**: 单个日志文件最大 10MB，保留 3 个文件

### 环境变量配置

如需覆盖配置文件中的设置，可在 `docker-compose.yml` 中的 `environment` 部分添加：

```yaml
environment:
  KIEAI_BASE_URL: https://api.kie.ai
  STORAGE_BASE_URL: http://your-domain.com/images
  STORAGE_LOCAL_PATH: /tmp/images
  TZ: Asia/Shanghai
```

### 生产环境建议

1. **使用外部配置文件**

```yaml
volumes:
  - /path/to/production/config.yaml:/app/config/config.yaml:ro
```

2. **持久化图片到主机目录**

```yaml
volumes:
  - /data/goloop/images:/tmp/images
```

3. **配置反向代理**

使用 Nginx 或 Traefik 作为反向代理，启用 HTTPS。

---

## GitHub Actions CI/CD

### 功能特性

工作流文件位于 `.github/workflows/docker-build.yml`，提供以下功能：

- ✅ 自动构建多平台镜像（amd64, arm64）
- ✅ 推送到 GitHub Container Registry (GHCR)
- ✅ 自动生成语义化版本标签
- ✅ 构建缓存加速
- ✅ 安全漏洞扫描（Trivy）

### 触发条件

| 事件 | 分支/标签 | 动作 |
|------|-----------|------|
| Push | master/main | 构建并推送 `latest` 标签 |
| Push | v1.2.3 格式的 tag | 构建并推送版本标签 |
| Pull Request | master/main | 仅构建，不推送 |

### 配置步骤

#### 1. 启用 GitHub Container Registry（默认启用）

无需额外配置，使用 `GITHUB_TOKEN` 自动推送到 `ghcr.io`。

#### 2. 发布版本

创建带版本号的 Git tag 即可触发构建：

```bash
# 创建版本标签
git tag v1.0.0
git push origin v1.0.0

# 工作流会自动构建以下标签：
# - v1.0.0
# - v1.0
# - v1
# - latest (如果是默认分支)
```

### 镜像标签说明

| 标签格式 | 示例 | 说明 |
|---------|------|------|
| `latest` | `latest` | 主分支最新构建 |
| 完整版本 | `v1.2.3` | 语义化版本号 |
| 主版本号 | `v1` | 主版本最新 |
| 次版本号 | `v1.2` | 次版本最新 |
| 分支+SHA | `main-abc1234` | 分支名+短 commit SHA |

### 拉取镜像

```bash
# 从 GitHub Container Registry 拉取
docker pull ghcr.io/<你的用户名>/goloop:latest

# 拉取特定版本
docker pull ghcr.io/<你的用户名>/goloop:v1.0.0
```

### 本地测试工作流

使用 [act](https://github.com/nektos/act) 在本地测试 GitHub Actions：

```bash
# 安装 act
brew install act

# 测试构建工作流
act push -j build

# 测试带 secrets
act push -j build --secret-file .secrets
```

---

## 手动 Docker 部署

### 构建镜像

```bash
# 单平台构建
docker build -t goloop:latest .

# 多平台构建（需要 buildx）
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t goloop:latest \
  --load \
  .
```

### 运行容器

```bash
docker run -d \
  --name goloop \
  -p 8080:8080 \
  -v $(pwd)/config/config.yaml:/app/config/config.yaml:ro \
  -v goloop-images:/tmp/images \
  --restart unless-stopped \
  goloop:latest
```

### 查看日志

```bash
# 实时查看日志
docker logs -f goloop

# 查看最近 100 行
docker logs --tail 100 goloop
```

### 健康检查

```bash
# 检查容器状态
docker ps

# 手动健康检查
curl http://localhost:8080/health
```

---

## 生产环境最佳实践

### 1. 使用具名标签

避免使用 `latest` 标签，使用具体版本号：

```yaml
services:
  goloop:
    image: ghcr.io/your-username/goloop:v1.0.0
```

### 2. 资源限制

```yaml
services:
  goloop:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

### 3. 配置监控

```yaml
services:
  goloop:
    labels:
      - "prometheus.scrape=true"
      - "prometheus.port=8080"
      - "prometheus.path=/metrics"
```

### 4. 配置网络隔离

```yaml
networks:
  backend:
    driver: bridge
    internal: true

services:
  goloop:
    networks:
      - backend
```

### 5. 定期更新镜像

```bash
# 拉取最新镜像
docker-compose pull

# 重启服务
docker-compose up -d
```

---

## 故障排查

### 容器无法启动

```bash
# 查看详细日志
docker-compose logs --tail=50 goloop

# 检查配置文件
docker-compose config

# 检查端口占用
lsof -i :8080
```

### 健康检查失败

```bash
# 进入容器检查
docker exec -it goloop sh

# 手动测试健康检查
wget -qO- http://localhost:8080/health
```

### 镜像推送失败

```bash
# 检查 GHCR 登录状态
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

---

## 更多资源

- [Docker Compose 文档](https://docs.docker.com/compose/)
- [GitHub Actions 文档](https://docs.github.com/actions)
- [GitHub Container Registry](https://docs.github.com/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
