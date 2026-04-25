#!/bin/bash
# 数据库管理脚本

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

case "$1" in
  init)
    echo "🔧 Initializing database..."
    go run scripts/init_db.go
    ;;
    
  seed)
    echo "🌱 Initializing database with seed data..."
    go run scripts/init_db.go --seed
    ;;
    
  check)
    echo "🔍 Checking database health..."
    go run scripts/init_db.go --check
    ;;
    
  clean)
    echo "🧹 Cleaning old logs..."
    go run scripts/init_db.go --clean
    ;;
    
  reset)
    echo "⚠️  WARNING: This will DROP ALL TABLES!"
    read -p "Are you sure? (yes/no): " confirm
    if [ "$confirm" = "yes" ]; then
      echo "🗑️  Dropping all tables..."
      psql "$DATABASE_URL" -c "DROP TABLE IF EXISTS usage_logs CASCADE;"
      psql "$DATABASE_URL" -c "DROP TABLE IF EXISTS api_keys CASCADE;"
      psql "$DATABASE_URL" -c "DROP TABLE IF EXISTS model_mappings CASCADE;"
      psql "$DATABASE_URL" -c "DROP TABLE IF EXISTS accounts CASCADE;"
      psql "$DATABASE_URL" -c "DROP TABLE IF EXISTS channels CASCADE;"
      echo "✅ Tables dropped"
      echo "🔧 Reinitializing..."
      go run scripts/init_db.go
    else
      echo "Cancelled."
    fi
    ;;
    
  backup)
    BACKUP_FILE="backup_$(date +%Y%m%d_%H%M%S).sql"
    echo "💾 Creating backup: $BACKUP_FILE"
    pg_dump "$DATABASE_URL" > "$BACKUP_FILE"
    echo "✅ Backup created: $BACKUP_FILE"
    ;;
    
  restore)
    if [ -z "$2" ]; then
      echo "Usage: $0 restore <backup_file>"
      exit 1
    fi
    echo "📥 Restoring from: $2"
    psql "$DATABASE_URL" < "$2"
    echo "✅ Database restored"
    ;;
    
  stats)
    echo "📊 Database statistics:"
    go run scripts/init_db.go --check
    ;;
    
  *)
    echo "Usage: $0 {init|seed|check|clean|reset|backup|restore|stats}"
    echo ""
    echo "Commands:"
    echo "  init      - Initialize database schema (create tables)"
    echo "  seed      - Initialize with seed data"
    echo "  check     - Check database health and show stats"
    echo "  clean     - Clean old usage logs (30+ days)"
    echo "  reset     - Drop all tables and reinitialize (WARNING: destroys data)"
    echo "  backup    - Create database backup"
    echo "  restore   - Restore from backup file"
    echo "  stats     - Show database statistics"
    exit 1
    ;;
esac
