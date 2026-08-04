#!/usr/bin/env bash
# record-operation.sh — 记录操作结果，更新短期记忆和统计
# 用法: bash record-operation.sh [项目根目录] [操作类型] [目标服务] [目标路径] [结果(success/failed)] [详情]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"
OP_TYPE="${2:-unknown}"
SERVICE="${3:-unknown}"
TARGET_DIR="${4:-unknown}"
RESULT="${5:-success}"
DETAIL="${6:-}"

if [ ! -d "$MEMORY_DIR" ]; then
  echo "[memory] 错误: .memory/ 目录不存在"
  exit 1
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")

OPS_FILE="$MEMORY_DIR/short-term/recent-operations.md"
FM_END=$(grep -n '^---' "$OPS_FILE" | tail -1 | cut -d: -f1)

OP_ID="OP-$(date -u +"%Y%m%d%H%M%S")"

NEW_OP="
## $OP_ID
- **类型**: $OP_TYPE
- **服务**: $SERVICE
- **目标路径**: $TARGET_DIR
- **结果**: $RESULT
- **详情**: $DETAIL
- **时间**: $NOW
"

echo "$NEW_OP" >> "$OPS_FILE"

OP_COUNT=$(grep -c '^## OP-' "$OPS_FILE" || echo "0")
if [ "$OP_COUNT" -gt 20 ]; then
  TEMP_FILE=$(mktemp)
  head -n "$FM_END" "$OPS_FILE" > "$TEMP_FILE"
  grep -A 6 '^## OP-' "$OPS_FILE" | tac | head -n 140 | tac >> "$TEMP_FILE"
  mv "$TEMP_FILE" "$OPS_FILE"
fi

sed -i "s/^updated:.*/updated: $NOW/" "$OPS_FILE"

STATS_FILE="$MEMORY_DIR/meta/stats.md"

TOTAL=$(grep '总操作次数:' "$STATS_FILE" | sed 's/.*: *//' | tr -d ' ')
SUCCESS=$(grep '成功次数:' "$STATS_FILE" | sed 's/.*: *//' | tr -d ' ')
FAILED=$(grep '失败次数:' "$STATS_FILE" | sed 's/.*: *//' | tr -d ' ')

TOTAL=$((TOTAL + 1))
if [ "$RESULT" = "success" ]; then
  SUCCESS=$((SUCCESS + 1))
else
  FAILED=$((FAILED + 1))
fi

if [ "$TOTAL" -gt 0 ]; then
  RATE=$(awk "BEGIN {printf \"%.1f\", ($SUCCESS / $TOTAL) * 100}")
else
  RATE="N/A"
fi

sed -i "s/总操作次数:.*/总操作次数: $TOTAL/" "$STATS_FILE"
sed -i "s/成功次数:.*/成功次数: $SUCCESS/" "$STATS_FILE"
sed -i "s/失败次数:.*/失败次数: $FAILED/" "$STATS_FILE"
sed -i "s/成功率:.*/成功率: ${RATE}%/" "$STATS_FILE"
sed -i "s/^updated:.*/updated: $NOW/" "$STATS_FILE"

if [ "$RESULT" = "failed" ]; then
  BUG_FILE="$MEMORY_DIR/long-term/bug-patterns.md"
  if [ -f "$BUG_FILE" ]; then
    if echo "$DETAIL" | grep -qi "being used by another process\|cannot access the file"; then
      echo "[memory] ⚠ 匹配已知 Bug 模式: BUG-001 (Windows 文件占用)"
    fi
    if echo "$DETAIL" | grep -qi "越界\|path.*\.\."; then
      echo "[memory] ⚠ 匹配已知 Bug 模式: BUG-002 (路径穿越)"
    fi
  fi
fi

sed -i "s/总操作次数:.*/总操作次数: $TOTAL/" "$MEMORY_DIR/index.md"
sed -i "s/成功率:.*/成功率: ${RATE}%/" "$MEMORY_DIR/index.md"

echo "[memory] 操作已记录: $OP_ID ($OP_TYPE → $RESULT)"
