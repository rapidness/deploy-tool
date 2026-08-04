# memory

**Description:** AI 多层记忆系统，支持跨会话上下文保持、经验积累、Bug 模式记录和操作习惯学习。纯规则/统计驱动，Markdown 格式存储。

## 记忆系统架构

四层记忆模型：
1. **工作记忆** (`.memory/working.md`) — 当前会话上下文，会话结束归档
2. **短期记忆** (`.memory/short-term/`) — 最近 20 条会话/操作记录，FIFO 淘汰
3. **长期记忆** (`.memory/long-term/`) — 项目上下文、经验、Bug 模式、操作习惯
4. **元记忆** (`.memory/meta/`) — 统计数据、记忆间关联关系

## 使用方法

### 首次使用（初始化）

```bash
bash .agents/skills/memory/scripts/init-memory.sh
```

### 会话生命周期

**会话开始时**（自动触发）：
```bash
bash .agents/skills/memory/scripts/session-start.sh . "当前任务描述"
```

**会话结束时**（自动触发）：
```bash
bash .agents/skills/memory/scripts/session-end.sh . "结果描述" "耗时"
```

### 记录操作

每次部署/服务控制等操作完成后：
```bash
bash .agents/skills/memory/scripts/record-operation.sh . "deploy" "MyAppService" "app/latest" "success" "已覆盖3个文件"
```

参数：`操作类型` `目标服务` `目标路径` `结果(success/failed)` `详情`

### 记录经验

获得重要经验教训时：
```bash
bash .agents/skills/memory/scripts/record-experience.sh . "标题" "问题" "原因" "解决" "代码位置" "标签1,标签2"
```

### 记录 Bug 模式

发现并修复 Bug 时：
```bash
bash .agents/skills/memory/scripts/record-bug.sh . "标题" "症状" "根因" "修复" "代码位置" "检测模式" "标签1,标签2"
```

### 更新项目上下文

项目结构发生重大变更时：
```bash
bash .agents/skills/memory/scripts/update-context.sh .
```

## 自动触发时机

| 时机 | 脚本 |
|------|------|
| 会话首条消息 | `session-start.sh` |
| 部署操作完成 | `record-operation.sh` |
| Bug 修复完成 | `record-bug.sh` |
| 获得重要经验 | `record-experience.sh` |
| 会话即将结束 | `session-end.sh` |

## 检索策略

按优先级检索记忆：
1. `working.md` → 当前会话上下文（最高优先）
2. `short-term/` → 近期相关记录
3. `long-term/` → 按类别和标签匹配
4. `meta/stats.md` → 统计数据辅助决策

## 记忆文件说明

| 文件 | 用途 | 生命周期 |
|------|------|---------|
| `working.md` | 当前会话任务、发现、假设 | 单次会话 |
| `short-term/recent-sessions.md` | 最近 20 次会话摘要 | FIFO 淘汰 |
| `short-term/recent-operations.md` | 最近 20 次操作记录 | FIFO 淘汰 |
| `long-term/project-context.md` | 项目结构、技术栈、规范 | 永久 |
| `long-term/experiences.md` | 经验教训、最佳实践 | 永久 |
| `long-term/bug-patterns.md` | Bug 模式、修复策略 | 永久 |
| `long-term/habits.md` | 操作习惯、偏好 | 永久 |
| `meta/stats.md` | 操作统计、成功率 | 永久 |
| `meta/relations.md` | 记忆间关联 | 永久 |
| `index.md` | 全局索引 | 永久 |
