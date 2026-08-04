#!/usr/bin/env bash
# update-context.sh — 扫描项目变更，更新项目上下文记忆
# 用法: bash update-context.sh [项目根目录]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"
CTX_FILE="$MEMORY_DIR/long-term/project-context.md"

if [ ! -d "$MEMORY_DIR" ]; then
  echo "[memory] 错误: .memory/ 目录不存在"
  exit 1
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")
TODAY=$(date -u +"%Y-%m-%d")

echo "[memory] 扫描项目变更..."

GO_FILES=$(find "$ROOT" -name "*.go" -not -path "*/vendor/*" -not -path "*/.git/*" | sort)
GO_COUNT=$(echo "$GO_FILES" | grep -c '\.go$' || echo "0")

WEB_FILES=$(find "$ROOT/web" -type f -not -path "*/.git/*" 2>/dev/null | sort)
WEB_COUNT=$(echo "$WEB_FILES" | grep -c '.' || echo "0")

GO_PACKAGES=$(find "$ROOT/internal" -name "*.go" -not -name "*_test.go" -not -path "*/vendor/*" 2>/dev/null | sed 's|/[^/]*\.go$||' | sort -u)

STRUCTURE=""
for pkg in $GO_PACKAGES; do
  REL_PKG=$(echo "$pkg" | sed "s|^$ROOT/||")
  GO_IN_PKG=$(find "$pkg" -maxdepth 1 -name "*.go" | sort)
  for f in $GO_IN_PKG; do
    REL_F=$(echo "$f" | sed "s|^$ROOT/||")
    STRUCTURE="$STRUCTURE$REL_F\n"
  done
done

if [ -d "$ROOT/web" ]; then
  WEB_STRUCTURE=$(find "$ROOT/web" -type f -not -path "*/.git/*" | sed "s|^$ROOT/||" | sort)
  for f in $WEB_STRUCTURE; do
    STRUCTURE="$STRUCTURE$f\n"
  done
fi

if [ -d "$ROOT/cmd" ]; then
  CMD_STRUCTURE=$(find "$ROOT/cmd" -type f -name "*.go" | sed "s|^$ROOT/||" | sort)
  for f in $CMD_STRUCTURE; do
    STRUCTURE="$STRUCTURE$f\n"
  done
fi

GO_VERSION="unknown"
if [ -f "$ROOT/go.mod" ]; then
  GO_VERSION=$(grep '^go ' "$ROOT/go.mod" | awk '{print $2}')
fi

DEPENDENCIES=""
if [ -f "$ROOT/go.mod" ]; then
  DEPENDENCIES=$(grep '^require' -A 100 "$ROOT/go.mod" | grep -v '^require\|^)$\|^$' | sed 's/ *\/\/.*//' | awk '{print "- " $1 " " $2}' || true)
fi

cat > "$CTX_FILE" << EOF
---
id: LT-001
category: project-context
tags: [go, windows, web, deploy]
updated: $TODAY
---

## 技术栈
- 后端: Go $GO_VERSION (单静态二进制, 零运行时依赖)
- 前端: 原生 HTML + htmx + 轻量 CSS (embed 编译进二进制)
- 配置: YAML (gopkg.in/yaml.v3)
- 认证: SHA-256 + Cookie 会话

## 项目结构
$(echo -e "$STRUCTURE" | head -50)

## 外部依赖
$DEPENDENCIES

## 文件统计
- Go 文件: $GO_COUNT 个
- 前端文件: $WEB_COUNT 个

## 变更检测
- 上次扫描: $NOW
EOF

sed -i "s/^updated:.*/updated: $NOW/" "$MEMORY_DIR/index.md"

echo "[memory] 项目上下文已更新 (Go: $GO_COUNT, Web: $WEB_COUNT)"
