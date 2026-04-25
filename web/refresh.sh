#!/bin/bash
# GoLoop 前端完全重建脚本

echo "🧹 清理所有缓存和构建文件..."
rm -rf .next out node_modules/.cache

echo "🔄 重启开发服务器..."
pkill -f "next dev" 2>/dev/null

echo "✅ 清理完成！"
echo ""
echo "请运行: npm run dev"
echo "然后在浏览器中按 Cmd+Shift+R 硬刷新"
