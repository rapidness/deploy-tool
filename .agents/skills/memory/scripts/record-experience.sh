#!/usr/bin/env bash
# record-experience.sh — 记录经验教训到长期记忆
# 用法: bash record-experience.sh [项目根目录] [标题] [问题] [原因] [解决] [代码位置] [标签(逗号分隔)]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"
TITLE="${2:-未命名经验}"
PROBLEM="${3:-未描述}"
CAUSE="${4:-未分析}"
SOLUTION="${5:-未记录}"
CODE_LOC="${6:-未记录}"
TAGS="${7:-}"

if [ ! -d "$MEMORY_DIR" ]; then
  echo "[memory] 错误: .memory/ 目录不存在"
  exit 1
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")
TODAY=$(date -u +"%Y-%m-%d")

EXP_FILE="$MEMORY_DIR/long-term/experiences.md"
EXISTING_COUNT=$(grep -c '^## EXP-' "$EXP_FILE" 2>/dev/null || echo "0")
NEW_ID="EXP-$(printf '%03d' $((EXISTING_COUNT + 1)))"

if [ -n "$TAGS" ]; then
  TAG_STR=$(echo "$TAGS" | tr ',' '\n' | xargs | sed 's/ /, /g')
else
  TAG_STR=""
fi

cat >> "$EXP_FILE" << EOF

---

## $NEW_ID: $TITLE

### 问题
$PROBLEM

### 原因
$CAUSE

### 解决
$SOLUTION

### 代码位置
- $CODE_LOC

### 教训
$PROBLEM 的根因是 $CAUSE，解决方案是 $SOLUTION
EOF

sed -i "s/^updated:.*/updated: $TODAY/" "$EXP_FILE"
if [ -n "$TAG_STR" ]; then
  sed -i "s/^tags:.*/tags: [$TAG_STR]/" "$EXP_FILE"
fi

sed -i "s/^updated:.*/updated: $NOW/" "$MEMORY_DIR/index.md"
TOTAL=$(grep 'total_entries:' "$MEMORY_DIR/index.md" | sed 's/.*: *//')
NEW_TOTAL=$((TOTAL + 1))
sed -i "s/total_entries:.*/total_entries: $NEW_TOTAL/" "$MEMORY_DIR/index.md"

echo "[memory] 经验已记录: $NEW_ID - $TITLE"
