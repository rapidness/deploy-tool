#!/usr/bin/env bash
# session-end.sh — 会话结束：归档 working.md 到 short-term，更新索引
# 用法: bash session-end.sh [项目根目录] [结果描述] [耗时]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"
RESULT="${2:-完成}"
DURATION="${3:-未知}"

if [ ! -d "$MEMORY_DIR" ]; then
  echo "[memory] 错误: .memory/ 目录不存在"
  exit 1
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")

SESSION_ID=$(grep '^session_id:' "$MEMORY_DIR/working.md" | sed 's/session_id: *"\(.*\)"/\1/')
TASK=$(grep '^-' "$MEMORY_DIR/working.md" | head -1 | sed 's/^- //')

SESSIONS_FILE="$MEMORY_DIR/short-term/recent-sessions.md"
FM_END=$(grep -n '^---' "$SESSIONS_FILE" | tail -1 | cut -d: -f1)

NEW_SESSION="
## S-$SESSION_ID
- **任务**: $TASK
- **结果**: $RESULT
- **耗时**: $DURATION
- **时间**: $NOW
"

echo "$NEW_SESSION" >> "$SESSIONS_FILE"

SESSION_COUNT=$(grep -c '^## S-' "$SESSIONS_FILE" || echo "0")
if [ "$SESSION_COUNT" -gt 20 ]; then
  TEMP_FILE=$(mktemp)
  head -n "$FM_END" "$SESSIONS_FILE" > "$TEMP_FILE"
  grep -A 5 '^## S-' "$SESSIONS_FILE" | tac | head -n 120 | tac >> "$TEMP_FILE"
  mv "$TEMP_FILE" "$SESSIONS_FILE"
fi

sed -i "s/^updated:.*/updated: $NOW/" "$SESSIONS_FILE"
sed -i 's/^status: active/status: completed/' "$MEMORY_DIR/working.md"
sed -i "s/^updated:.*/updated: $NOW/" "$MEMORY_DIR/index.md"
sed -i "s/最近会话:.*/最近会话: $SESSION_ID/" "$MEMORY_DIR/index.md"

echo "[memory] 会话结束: $SESSION_ID"
echo "[memory] 结果: $RESULT"
echo "[memory] 耗时: $DURATION"
echo "[memory] 已归档到 recent-sessions.md"
