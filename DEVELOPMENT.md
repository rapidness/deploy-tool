# 服务更新工具 - 开发说明文档

## 1. 系统简介

服务更新工具是一个部署在 **Windows 服务器**上的 Web 应用，用于将本地开发完成的更新文件（zip 压缩包）一键部署到服务器目标服务。核心流程为：**接收文件 → 停止服务 → 解压覆盖文件 → 启动服务**，全程通过浏览器操作，无需远程登录服务器。

### 技术栈

| 层  | 技术                      | 说明                      |
| -- | ----------------------- | ----------------------- |
| 后端 | Go (Golang)             | 单静态二进制，零运行时依赖           |
| 前端 | 原生 HTML + htmx + 轻量 CSS | 无构建步骤，通过 `embed` 编译进二进制 |
| 配置 | YAML                    | `gopkg.in/yaml.v3`      |
| 认证 | SHA-256 + Cookie 会话     | 密码哈希存储，非明文              |

### 项目结构

```
update-app/
├── cmd/app/main.go              # 程序入口
├── internal/
│   ├── config/                  # 配置加载与校验
│   │   ├── config.go
│   │   └── config_test.go
│   ├── handler/                 # HTTP 路由与 API 处理
│   │   ├── handler.go
│   │   └── handler_test.go
│   ├── middleware/              # 认证中间件
│   │   ├── auth.go
│   │   └── auth_test.go
│   ├── service/                 # 业务逻辑（服务启停、zip 解压、部署流程）
│   │   ├── service.go
│   │   └── service_test.go
│   └── store/                   # 操作历史持久化
│       ├── store.go
│       └── store_test.go
├── web/
│   ├── embed.go                 # embed 声明
│   └── static/                  # 前端静态资源
│       ├── index.html           # 主页面
│       ├── login.html           # 登录页
│       ├── style.css            # 样式
│       └── htmx.min.js          # htmx 库
├── config.example.yaml          # 配置文件模板
├── go.mod / go.sum              # 依赖管理
├── .gitignore
└── requirements.md              # 需求文档
```

## 2. 环境准备

### 2.1 安装 Go

需 Go 1.16+（本项目使用 `embed` 包），推荐 Go 1.21+。

下载地址：<https://go.dev/dl/>

安装后验证：

```bash
go version
# go version go1.26.2 windows/amd64
```

### 2.2 获取依赖

在项目根目录执行：

```bash
go mod tidy
```

唯一外部依赖为 `gopkg.in/yaml.v3`，用于解析 YAML 配置文件。

## 3. 配置文件

### 3.1 创建配置文件

复制模板并编辑：

```bash
cp config.example.yaml config.yaml
```

### 3.2 配置项说明

```yaml
server:
  host: "0.0.0.0"       # 监听地址（0.0.0.0=所有网卡，127.0.0.1=仅本机）
  port: 8080             # 监听端口

auth:
  username: "admin"      # 登录用户名
  password: "哈希值"     # 密码的 SHA-256 哈希（非明文）

root_dir: "C:\\Services" # 可更新的根目录（安全边界，更新不得越出此目录）

services:                 # 被管理的 Windows 服务列表
  - name: "MyAppService"           # 服务名称（sc.exe 中注册的名称）
    display_name: "我的应用服务"    # 界面显示名称
  - name: "AnotherService"
    display_name: "另一个服务"

log:
  path: "logs"            # 日志与历史记录存储路径
  max_size_mb: 10         # 单个日志文件最大体积（MB）
  max_backups: 5          # 保留的旧日志文件数量
  max_age_days: 30        # 日志最大保留天数
```

### 3.3 生成密码哈希

配置文件中的 `password` 字段存储的是 SHA-256 哈希值，**不是明文密码**。使用以下命令生成：

```bash
# 方式一：编译后使用
app.exe -hash-password yourpassword
# 输出: 5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1546d8

# 方式二：开发时直接运行
go run ./cmd/app -hash-password yourpassword
```

将输出的哈希值填入 `config.yaml` 的 `auth.password` 字段。

> 模板中默认哈希 `8c6976e5...` 对应明文密码 `admin`，**生产环境务必修改**。

## 4. 运行与调试

### 4.1 直接运行（开发模式）

```bash
# 使用默认配置文件 config.yaml
go run ./cmd/app

# 指定配置文件
go run ./cmd/app -config /path/to/config.yaml
```

启动后控制台输出：

```
服务更新工具启动，监听 http://0.0.0.0:8080
```

浏览器访问 `http://localhost:8080` 即可。

### 4.2 运行编译后的程序

```bash
# 先编译（见第 5 节）
go build -o app.exe ./cmd/app

# 运行
./app.exe -config config.yaml
```

### 4.3 停止服务

### 4.4 调试技巧

**热重载**（推荐安装 [air](https://github.com/cosmtrek/air)）：

```bash
# 安装 air
go install github.com/cosmtrek/air@latest

# 在项目根目录运行
air
```

修改代码后自动重新编译并重启。

**手动调试**：

```bash
# 编译带调试信息的二进制
go build -gcflags="all=-N -l" -o app.exe ./cmd/app

# 使用 Delve 调试
dlv exec ./app.exe -- -config config.yaml
```

**日志查看**：

程序使用标准库 `log` 输出到控制台（stdout），操作历史记录在 `logs/history.json`。

### 4.5 命令行参数

| 参数               | 说明                    | 默认值           |
| ---------------- | --------------------- | ------------- |
| `-config`        | 配置文件路径                | `config.yaml` |
| `-hash-password` | 计算指定密码的 SHA-256 哈希并退出 | 无             |

## 5. 编译打包

### 5.1 基本编译


# 编译为当前平台的可执行文件
```bash
go build -o app.exe ./cmd/app
```

产物 `app.exe` 约 10\~15MB，已包含全部前端资源（通过 `embed`）。

### 5.2 交叉编译

在任意操作系统上编译 Windows 版本：

```bash
# Linux/Mac 上编译 Windows exe
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o app.exe ./cmd/app

# 编译 32 位版本
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -o app.exe ./cmd/app
```

`CGO_ENABLED=0` 确保纯静态编译，无外部 C 库依赖。

### 5.3 减小体积

```bash
# 去除调试信息与符号表（体积可减小约 30%）
go build -ldflags="-s -w" -o app.exe ./cmd/app
```

| 方式                 | 体积（参考） |
| ------------------ | ------ |
| 默认编译               | \~15MB |
| `-ldflags="-s -w"` | \~10MB |

### 5.4 编译验证

提交前确保通过：

```bash
go build ./...     # 全部包可编译
go vet ./...       # 静态检查无警告
go test ./...      # 全部测试通过
```

## 6. 测试

### 6.1 运行全部测试

```bash
go test ./...
```

### 6.2 运行指定模块测试

```bash
go test ./internal/config/...    # 配置模块
go test ./internal/service/...   # 业务逻辑模块
go test ./internal/store/...     # 存储模块
go test ./internal/middleware/... # 认证模块
go test ./internal/handler/...   # HTTP 处理模块
```

### 6.3 查看详细输出

```bash
go test ./... -v
```

### 6.4 生成覆盖率报告

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
# 浏览器打开 coverage.html 查看覆盖率
```

### 6.5 测试内容概览

| 模块         | 测试用例数 | 覆盖内容                           |
| ---------- | ----- | ------------------------------ |
| config     | 7     | 合法/非法配置、端口校验、空认证、空服务、服务查找      |
| store      | 5     | 增删查、排序、limit、文件持久化与重载          |
| service    | 10    | zip 校验、路径安全（越界/绝对路径）、解压覆盖、目录条目 |
| middleware | 6     | 密码哈希、登录成功/失败、cookie 鉴权、API 401 |
| handler    | 1     | 辅助函数解析                         |

## 7. 部署

### 7.1 部署产物

部署只需两个文件：

```
app.exe          # 可执行文件（含前端资源）
config.yaml      # 配置文件
```

### 7.2 部署步骤

### 7.3 注册为 Windows 服务（可选）

如需开机自启，可使用 [NSSM](https://nssm.cc/) 将 `app.exe` 注册为 Windows 服务：

```bash
# 下载 nssm 后
nssm install UpdateApp C:\update-app\app.exe -config C:\update-app\config.yaml
nssm start UpdateApp
```

卸载：

```bash
nssm stop UpdateApp
nssm remove UpdateApp
```

### 7.4 防火墙配置

若需从其他机器访问，开放对应端口：

```powershell
# 以管理员身份运行 PowerShell
New-NetFirewallRule -DisplayName "UpdateApp" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

### 7.5 目录结构（运行时）

```
C:\update-app\
├── app.exe          # 可执行文件
├── config.yaml      # 配置文件
└── logs/            # 运行时自动生成
    └── history.json # 操作历史记录
```

## 8. 使用流程

### 8.1 典型操作步骤

### 8.2 手动控制服务

在「服务状态」区域，可对单个服务执行启动/停止/重启操作（不影响文件）。

### 8.3 失败处理

| 失败场景   | 系统行为       | 用户后续操作          |
| ------ | ---------- | --------------- |
| 停服务失败  | 中止，不覆盖文件   | 检查服务状态与权限       |
| 解压覆盖失败 | 中止，不启动服务   | 服务保持停止状态，用户手动处理 |
| 启动服务失败 | 提示错误，文件已覆盖 | 检查服务配置或手动启动     |

## 9. API 接口

所有 API 需携带登录 Cookie。

| 方法   | 路径                     | 说明                                          |
| ---- | ---------------------- | ------------------------------------------- |
| GET  | `/`                    | 主页面                                         |
| GET  | `/login`               | 登录页面                                        |
| POST | `/login`               | 提交登录                                        |
| POST | `/logout`              | 登出                                          |
| GET  | `/api/services`        | 获取所有服务状态                                    |
| POST | `/api/deploy`          | 执行更新（multipart：service + file）              |
| GET  | `/api/history`         | 获取操作历史（最近 50 条）                             |
| POST | `/api/service/control` | 手动控制服务（service + action=start/stop/restart） |

### 示例：执行更新

```bash
curl -X POST http://localhost:8080/api/deploy \
  -F "service=MyAppService" \
  -F "file=@update.zip" \
  -b "update_app_session=会话cookie"
```

响应：

```json
{
  "Steps": [
    {"name": "stop", "status": "success", "message": "服务已停止", "duration": 1200},
    {"name": "deploy", "status": "success", "message": "已覆盖 3 个文件", "duration": 800},
    {"name": "start", "status": "success", "message": "服务已启动", "duration": 2100}
  ],
  "FinalState": "success"
}
```

## 10. 安全注意事项

## 11. 常见问题

### Q: 启动时报 "root\_dir 不存在"？

A: 配置文件中 `root_dir` 指定的目录必须真实存在，请先在服务器上创建该目录。

### Q: 服务启停失败？

A: 需以**管理员权限**运行 `app.exe`，才能通过 `sc.exe` 控制 Windows 服务。

### Q: 上传大文件失败？

A: 系统默认限制单文件 500MB，如需调整请修改 `handler.go` 中 `maxSize` 常量。

### Q: 如何在非 Windows 系统上开发？

A: 代码可在 Linux/Mac 上编译和运行测试（`go test ./...`），但服务启停功能依赖 Windows 的 `sc.exe`，仅在实际 Windows 环境中有效。

### Q: 前端如何修改？

A: 直接编辑 `web/static/` 下的 HTML/CSS/JS 文件，重新 `go build` 即可。无需前端构建工具。如需使用完整 htmx 功能，请从 <https://htmx.org/> 下载 `htmx.min.js` 替换占位文件。
