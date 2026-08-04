#!/usr/bin/env bash
# session-start.sh — 会话开始：清空 working.md，生成新会话头，读取索引摘要
# 用法: bash session-start.sh [项目根目录] [会话描述]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"
DESCRIPTION="${2:-未指定}"

if [ ! -d "$MEMORY_DIR" ]; then
  echo "[memory] 错误: .memory/ 目录不存在，请先运行 init-memory.sh"
  exit 1
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")
SESSION_ID="$(date -u +"%Y-%m-%d")-$(printf '%03d' $$)"

cat > "$MEMORY_DIR/working.md" << EOF
---
session_id: "$SESSION_ID"
started: $NOW
status: active
---

## 当前任务
- $DESCRIPTION

## 临时发现

## 待验证
EOF

echo "[memory] 会话开始: $SESSION_ID"
echo "[memory] 任务: $DESCRIPTION"

if [ -f "$MEMORY_DIR/index.md" ]; then
  echo ""
  echo "[memory] === 索引摘要 ==="
  sed -n '/^## 统计摘要/,/^$/p' "$MEMORY_DIR/index.md" | head -10
  echo ""
  echo "[memory] === 近期记忆 ==="
  sed -n '/^## 记忆清单/,/^$/p' "$MEMORY_DIR/index.md" | head -8
fi

echo ""
echo "[memory] 提示: 使用 update-context.sh 更新项目上下文"
