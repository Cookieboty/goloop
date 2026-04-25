#!/bin/bash
# 初始化开发环境脚本

set -e

echo "🚀 GoLoop 开发环境初始化"
echo "================================"

# 检查必需的命令
check_command() {
    if ! command -v $1 &> /dev/null; then
        echo "❌ 错误: $1 未安装"
        echo "   请安装 $1: $2"
        exit 1
    fi
}

echo ""
echo "📋 检查依赖..."
check_command "psql" "brew install postgresql"
check_command "redis-cli" "brew install redis"
check_command "go" "brew install go"
check_command "node" "brew install node"
echo "✅ 所有依赖已安装"

echo ""
echo "🗄️  设置 PostgreSQL 数据库..."

# 从 .env 文件读取数据库配置
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# 提取数据库信息
DB_NAME=${DATABASE_URL##*/}
DB_NAME=${DB_NAME%%\?*}

# 检查数据库是否存在
if psql -lqt | cut -d \| -f 1 | grep -qw "$DB_NAME"; then
    echo "✅ 数据库 '$DB_NAME' 已存在"
else
    echo "📦 创建数据库 '$DB_NAME'..."
    createdb "$DB_NAME"
    echo "✅ 数据库创建成功"
fi

echo ""
echo "🔧 初始化数据库表..."
go run scripts/init_db.go
echo "✅ 数据库表初始化完成"

echo ""
echo "📊 Redis 服务检查..."
if redis-cli ping > /dev/null 2>&1; then
    echo "✅ Redis 服务正常运行"
else
    echo "⚠️  Redis 未运行"
    echo "   启动 Redis: brew services start redis"
    echo "   或者临时运行: redis-server"
fi

echo ""
echo "📦 安装前端依赖..."
cd web && npm install && cd ..
echo "✅ 前端依赖安装完成"

echo ""
echo "================================"
echo "✅ 开发环境设置完成！"
echo ""
echo "下一步："
echo "  1. 启动服务: make dev"
echo "  2. 访问 Admin UI: http://localhost:8080/admin/ui/"
echo "  3. 使用密码登录: \$ADMIN_PASSWORD"
echo "  4. 在 UI 中配置渠道和 API Key"
echo ""
