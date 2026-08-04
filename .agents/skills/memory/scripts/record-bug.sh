#!/usr/bin/env bash
# record-bug.sh — 记录 Bug 模式到长期记忆
# 用法: bash record-bug.sh [项目根目录] [标题] [症状] [根因] [修复] [代码位置] [检测模式] [标签(逗号分隔)]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"
TITLE="${2:-未命名Bug}"
SYMPTOM="${3:-未描述}"
ROOT_CAUSE="${4:-未分析}"
FIX="${5:-未修复}"
CODE_LOC="${6:-未记录}"
DETECT_PATTERN="${7:-未记录}"
TAGS="${8:-}"

if [ ! -d "$MEMORY_DIR" ]; then
  echo "[memory] 错误: .memory/ 目录不存在"
  exit 1
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")
TODAY=$(date -u +"%Y-%m-%d")

BUG_FILE="$MEMORY_DIR/long-term/bug-patterns.md"
EXISTING_COUNT=$(grep -c '^## BUG-' "$BUG_FILE" 2>/dev/null || echo "0")
NEW_ID="BUG-$(printf '%03d' $((EXISTING_COUNT + 1)))"

if [ -n "$TAGS" ]; then
  TAG_STR=$(echo "$TAGS" | tr ',' '\n' | xargs | sed 's/ /, /g')
else
  TAG_STR=""
fi

cat >> "$BUG_FILE" << EOF

---

## $NEW_ID: $TITLE

### 症状
$SYMPTOM

### 根因
$ROOT_CAUSE

### 修复
$FIX

### 代码位置
- $CODE_LOC

### 检测模式
$DETECT_PATTERN
EOF

sed -i "s/^updated:.*/updated: $TODAY/" "$BUG_FILE"
if [ -n "$TAG_STR" ]; then
  sed -i "s/^tags:.*/tags: [$TAG_STR]/" "$BUG_FILE"
fi

RELATIONS_FILE="$MEMORY_DIR/meta/relations.md"
EXP_FILE="$MEMORY_DIR/long-term/experiences.md"
if [ -f "$EXP_FILE" ]; then
  KEYWORDS=$(echo "$TITLE" | tr ' ' '\n' | grep -v '^\|$' | head -3)
  for KW in $KEYWORDS; do
    MATCHING_EXP=$(grep "^## EXP-" "$EXP_FILE" | grep -i "$KW" | head -1 | sed 's/## \(EXP-[0-9]*\).*/\1/')
    if [ -n "$MATCHING_EXP" ]; then
      echo "- $NEW_ID → $MATCHING_EXP (Bug 催生经验)" >> "$RELATIONS_FILE"
      echo "[memory] 建立关联: $NEW_ID → $MATCHING_EXP"
    fi
  done
fi
sed -i "s/^updated:.*/updated: $TODAY/" "$RELATIONS_FILE"

sed -i "s/^updated:.*/updated: $NOW/" "$MEMORY_DIR/index.md"
TOTAL=$(grep 'total_entries:' "$MEMORY_DIR/index.md" | sed 's/.*: *//')
NEW_TOTAL=$((TOTAL + 1))
sed -i "s/total_entries:.*/total_entries: $NEW_TOTAL/" "$MEMORY_DIR/index.md"

echo "[memory] Bug 模式已记录: $NEW_ID - $TITLE"
