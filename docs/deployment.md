# 服务更新工具 — 部署文档

## 1. 部署架构

```
浏览器/CI/CD ──HTTPS──▶ Nginx (:443/:80)
                              │
                              ├─ /update-module/ ──▶ app.exe (:8080)
                              ├─ /other-app/     ──▶ 其他子系统
                              └─ /               ──▶ 主应用
```

服务更新工具作为子系统，通过 Nginx 反向代理以子路径方式对外提供服务。

## 2. 部署步骤

### 2.1 编译

在任意平台交叉编译 Windows 可执行文件：

```bash
# 编译（本地 Windows 直接 build）
go build -o app.exe ./cmd/app

# 或从 Linux/Mac 交叉编译
GOOS=windows GOARCH=amd64 go build -o app.exe ./cmd/app
```

产物为单个 `app.exe`，内含前端资源，无需额外依赖。

### 2.2 目录结构

在服务器上创建部署目录：

```
C:\Services\update-app\
├── app.exe          # 可执行文件
├── config.yaml      # 配置文件
└── logs\            # 日志目录（自动创建）
```

### 2.3 配置文件

复制 `config.example.yaml` 为 `config.yaml`，按实际环境修改：

```yaml
server:
  host: "127.0.0.1"    # 仅监听本机（由 Nginx 代理）
  port: 8080

auth:
  username: "admin"
  password: "<使用 app.exe -hash-password 生成>"
  api_token: "<随机生成的Token，用于CI/CD调用>"

root_dir: "C:\\Services"

log_dir: "C:\\Services\\logs"

# 子路径前缀，必须与 Nginx 的 location 匹配
base_path: "/update-module"

services:
  - name: "MyAppService"
    display_name: "我的应用服务"

log:
  path: "logs"
  max_size_mb: 10
  max_backups: 5
  max_age_days: 30

rate_limit:
  enabled: true
  max_requests: 60
  per_ip: true
```

**关键配置说明**：

| 配置项 | 说明 |
|--------|------|
| `server.host` | 设为 `127.0.0.1`，仅本机访问，由 Nginx 代理对外暴露 |
| `base_path` | **必须与 Nginx 的 `location` 路径一致**，如 `/update-module` |
| `api_token` | 为空则不启用 Bearer 认证；建议生成随机字符串填入 |
| `root_dir` | 可更新的根目录，所有部署操作限于此目录内（安全边界） |

### 2.4 生成密码哈希

```powershell
.\app.exe -hash-password "你的密码"
# 输出: 8c6976e5b5410415bde908bd4dee15dfb167a9c873fc4bb8a81f6f2ab448a918
```

将输出填入 `config.yaml` 的 `auth.password`。

### 2.5 生成 API Token

```powershell
# PowerShell 生成 32 字节随机 Token
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Maximum 256 }) -as [byte[]])
```

将输出填入 `config.yaml` 的 `auth.api_token`。

### 2.6 启动服务

```powershell
# 前台启动（调试用）
.\app.exe -config config.yaml

# 后台启动（生产用）
Start-Process -FilePath ".\app.exe" -ArgumentList "-config","config.yaml" -WindowStyle Hidden
```

### 2.7 注册为 Windows 服务（可选）

使用 [NSSM](https://nssm.cc/) 将应用注册为 Windows 服务，实现开机自启和自动重启：

```powershell
nssm install UpdateApp "C:\Services\update-app\app.exe" "-config C:\Services\update-app\config.yaml"
nssm set UpdateApp AppDirectory "C:\Services\update-app"
nssm set UpdateApp DisplayName "服务更新工具"
nssm set UpdateApp Start SERVICE_AUTO_START
nssm start UpdateApp
```

## 3. Nginx 配置

### 3.1 子路径代理配置

服务更新工具作为子系统，通过 `/update-module/` 路径访问。在 Nginx 配置中添加：

```nginx
server {
    listen 80;
    server_name your-server.com;

    # 强制 HTTPS（生产环境推荐）
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name your-server.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # ============================================================
    # 服务更新工具 — 子路径代理
    # ============================================================
    location /update-module/ {
        proxy_pass http://127.0.0.1:8080/update-module/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 文件上传大小限制（与 app 的 500MB 限制匹配）
        client_max_body_size 500m;

        # 上传超时（大文件上传需要较长超时）
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;

        # WebSocket 支持（如未来需要）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # ============================================================
    # 其他子系统示例
    # ============================================================
    # location /other-app/ {
    #     proxy_pass http://127.0.0.1:8081/other-app/;
    #     ...
    # }

    # 主应用
    location / {
        root /var/www/html;
        index index.html;
    }
}
```

### 3.2 关键配置说明

| 配置项 | 说明 |
|--------|------|
| `location /update-module/` | 子路径，需与 `config.yaml` 的 `base_path` 一致 |
| `proxy_pass` | 末尾的 `/` 不可省略，确保路径正确转发 |
| `client_max_body_size` | 必须大于等于最大上传文件大小，默认 1m 太小 |
| `proxy_read_timeout` | 大文件上传+部署耗时较长，建议 300s |

### 3.3 base_path 与 Nginx 路径对应关系

| 场景 | `base_path` | Nginx `location` | 访问地址 |
|------|-------------|------------------|----------|
| 子路径部署 | `/update-module` | `/update-module/` | `https://host/update-module/` |
| 根路径部署 | `""` | `/` | `https://host/` |
| 其他子路径 | `/deploy` | `/deploy/` | `https://host/deploy/` |

**重要**：`base_path` 和 Nginx `location` 必须一致，否则静态资源和 API 路径会 404。

### 3.4 仅内网访问（无 HTTPS）

如果仅在内网使用，可以不配置 HTTPS：

```nginx
server {
    listen 80;
    server_name _;

    # 限制仅内网访问
    allow 192.168.0.0/16;
    allow 10.0.0.0/8;
    deny all;

    location /update-module/ {
        proxy_pass http://127.0.0.1:8080/update-module/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        client_max_body_size 500m;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
}
```

### 3.5 Nginx 日志配置（可选）

为服务更新工具单独记录访问日志：

```nginx
location /update-module/ {
    access_log /var/log/nginx/update-app-access.log combined;
    error_log  /var/log/nginx/update-app-error.log warn;

    proxy_pass http://127.0.0.1:8080/update-module/;
    # ... 其余配置同上
}
```

## 4. 验证部署

### 4.1 本地验证

```powershell
# 检查服务是否启动
curl http://127.0.0.1:8080/update-module/api/services -H "Authorization: Bearer YOUR_TOKEN"
```

### 4.2 通过 Nginx 验证

```powershell
# 检查 Nginx 代理是否正常
curl https://your-server.com/update-module/api/services -H "Authorization: Bearer YOUR_TOKEN"
```

### 4.3 浏览器访问

打开 `https://your-server.com/update-module/`，应显示登录页面。

## 5. 更新本工具自身

本工具自身的更新流程：

1. 编译新版本 `app.exe`
2. 停止当前运行的服务（关闭进程或 `nssm stop UpdateApp`）
3. 替换 `app.exe` 文件
4. 按需更新 `config.yaml`
5. 重新启动服务

## 6. 常见问题

### Q: 页面空白或 404

检查 `base_path` 与 Nginx `location` 是否一致。`base_path` 不带尾部 `/`，Nginx `location` 带尾部 `/`。

### Q: 上传大文件失败

1. 检查 Nginx 的 `client_max_body_size` 是否足够大
2. 检查 Nginx 的 `proxy_read_timeout` 是否足够长
3. 检查应用自身的 500MB 限制（`handler.go` 中 `maxSize`）

### Q: API Token 认证不生效

1. 确认 `config.yaml` 中 `auth.api_token` 不为空
2. 确认请求头格式为 `Authorization: Bearer <token>`（注意 Bearer 后有空格）
3. 检查 Nginx 是否正确转发 `Authorization` 头（默认会转发）

### Q: 限流 429 错误

1. 检查 `rate_limit.max_requests` 是否过小
2. 考虑 `per_ip: true` 模式下，Nginx 代理可能导致所有请求来自同一 IP，需配置 `X-Real-IP` 头
