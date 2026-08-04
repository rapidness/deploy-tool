#!/usr/bin/env bash
# init-memory.sh — 初始化 AI 多层记忆系统目录结构
# 用法: bash init-memory.sh [项目根目录]

set -euo pipefail

ROOT="${1:-.}"
MEMORY_DIR="$ROOT/.memory"

if [ -d "$MEMORY_DIR" ]; then
  echo "[memory] .memory/ 目录已存在，跳过初始化"
  exit 0
fi

echo "[memory] 初始化记忆系统..."

mkdir -p "$MEMORY_DIR/short-term"
mkdir -p "$MEMORY_DIR/long-term"
mkdir -p "$MEMORY_DIR/meta"

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S")
TODAY=$(date -u +"%Y-%m-%d")

cat > "$MEMORY_DIR/index.md" << EOF
---
updated: $NOW
version: 1
total_entries: 0
---

# 记忆索引

## 记忆清单
| ID | 层级 | 类别 | 标题 | 更新时间 | 优先级 |
|----|------|------|------|---------|--------|

## 统计摘要
- 总操作次数: 0
- 成功率: N/A
- 最常见失败: N/A
- 最活跃服务: N/A
- 最近会话: N/A
EOF

cat > "$MEMORY_DIR/working.md" << EOF
---
session_id: ""
started: ""
status: idle
---

## 当前任务

## 临时发现

## 待验证
EOF

cat > "$MEMORY_DIR/short-term/recent-sessions.md" << EOF
---
updated: $NOW
max_entries: 20
---
EOF

cat > "$MEMORY_DIR/short-term/recent-operations.md" << EOF
---
updated: $NOW
max_entries: 20
---
EOF

cat > "$MEMORY_DIR/long-term/project-context.md" << EOF
---
id: LT-001
category: project-context
tags: [go, windows, web, deploy]
updated: $TODAY
---

## 技术栈

## 项目结构

## 核心模块职责

## API 端点

## 代码规范
EOF

cat > "$MEMORY_DIR/long-term/experiences.md" << EOF
---
id: LT-002
category: experience
tags: []
updated: $TODAY
---
EOF

cat > "$MEMORY_DIR/long-term/bug-patterns.md" << EOF
---
id: LT-003
category: bug-pattern
tags: []
updated: $TODAY
---
EOF

cat > "$MEMORY_DIR/long-term/habits.md" << EOF
---
id: LT-004
category: habit
tags: [部署, 偏好]
updated: $TODAY
---

## 常用部署组合

## 操作偏好

## 快捷路径
EOF

cat > "$MEMORY_DIR/meta/stats.md" << EOF
---
updated: $NOW
---

## 操作统计
- 总操作次数: 0
- 成功次数: 0
- 失败次数: 0
- 成功率: N/A

## 失败模式排名
| 排名 | 失败类型 | 次数 | 最近发生 |
|------|---------|------|---------|

## 服务活跃度
| 服务 | 操作次数 | 成功率 |
|------|---------|--------|

## 时间分布
EOF

cat > "$MEMORY_DIR/meta/relations.md" << EOF
---
updated: $TODAY
---

## 记忆关联

### 因果链

### 主题关联

### 引用图
EOF

if [ -f "$ROOT/.gitignore" ]; then
  if ! grep -q "^\.memory/" "$ROOT/.gitignore"; then
    echo "" >> "$ROOT/.gitignore"
    echo "# AI 记忆系统" >> "$ROOT/.gitignore"
    echo ".memory/" >> "$ROOT/.gitignore"
    echo "[memory] 已追加 .memory/ 到 .gitignore"
  fi
else
  echo ".memory/" > "$ROOT/.gitignore"
  echo "[memory] 已创建 .gitignore 并添加 .memory/"
fi

echo "[memory] 初始化完成！目录结构："
find "$MEMORY_DIR" -type f | sort
