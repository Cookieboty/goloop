#!/bin/bash
# GoLoop 启动脚本 - 自动加载 .env 文件

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 加载 .env 文件
if [ -f .env ]; then
    echo "📝 Loading environment variables from .env..."
    set -a
    source .env
    set +a
else
    echo "❌ Error: .env file not found"
    echo "   Please copy .env.example to .env and configure it"
    exit 1
fi

# 检查必需的环境变量
if [ -z "$DATABASE_URL" ]; then
    echo "❌ Error: DATABASE_URL is not set"
    exit 1
fi

if [ -z "$JWT_SECRET" ]; then
    echo "❌ Error: JWT_SECRET is not set"
    exit 1
fi

if [ -z "$ADMIN_PASSWORD" ]; then
    echo "❌ Error: ADMIN_PASSWORD is not set"
    exit 1
fi

echo "✅ Environment variables loaded"
echo "🚀 Starting GoLoop..."
echo ""

# 运行服务
./bin/goloop
