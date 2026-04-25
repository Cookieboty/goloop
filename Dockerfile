# syntax=docker/dockerfile:1

# Stage 1: Build Next.js frontend
FROM node:22-alpine AS frontend

WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ .
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /web/out ./internal/admin/out

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /server ./cmd/server

# Stage 3: Minimal runtime image
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata postgresql-client netcat-openbsd

WORKDIR /app

COPY --from=builder /server /app/server

RUN mkdir -p /tmp/images

# 添加数据库连接等待脚本
COPY <<'EOF' /app/wait-for-db.sh
#!/bin/sh
set -e

echo "等待数据库连接..."

# 从 DATABASE_URL 解析连接信息
if [ -n "$DATABASE_URL" ]; then
    # 解析 postgresql://user:password@host:port/dbname
    DB_USER=$(echo $DATABASE_URL | sed -n 's/.*:\/\/\([^:]*\):.*/\1/p')
    DB_PASS=$(echo $DATABASE_URL | sed -n 's/.*:\/\/[^:]*:\([^@]*\)@.*/\1/p')
    DB_HOST=$(echo $DATABASE_URL | sed -n 's/.*@\([^:]*\):.*/\1/p')
    DB_PORT=$(echo $DATABASE_URL | sed -n 's/.*:\([0-9]*\)\/.*/\1/p')
    DB_NAME=$(echo $DATABASE_URL | sed -n 's/.*\/\([^?]*\).*/\1/p')
    
    export PGPASSWORD="$DB_PASS"
    
    until pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" > /dev/null 2>&1; do
        echo "PostgreSQL 还未就绪，等待中..."
        sleep 2
    done
    
    echo "PostgreSQL 已就绪！"
    
    # 测试连接和数据库
    until psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c '\q' > /dev/null 2>&1; do
        echo "数据库连接测试失败，重试中..."
        sleep 2
    done
    
    echo "数据库连接测试成功！"
fi

# 检查 Redis 连接（如果启用）
if [ "$REDIS_ENABLED" != "false" ] && [ -n "$REDIS_URL" ]; then
    echo "检查 Redis 连接..."
    REDIS_HOST=$(echo $REDIS_URL | sed -n 's/.*:\/\/\([^:]*\):.*/\1/p')
    REDIS_PORT=$(echo $REDIS_URL | sed -n 's/.*:\([0-9]*\).*/\1/p')
    
    until nc -z "$REDIS_HOST" "$REDIS_PORT" 2>/dev/null; do
        echo "Redis 还未就绪，等待中..."
        sleep 2
    done
    
    echo "Redis 已就绪！"
fi

echo "所有依赖服务已就绪，启动应用..."
exec "$@"
EOF

RUN chmod +x /app/wait-for-db.sh

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/wait-for-db.sh"]
CMD ["/app/server"]
