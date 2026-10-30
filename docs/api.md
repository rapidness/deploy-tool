# 服务更新工具 API 对接文档

## 1. 概述

服务更新工具提供 RESTful API，支持通过 HTTP 接口完成服务部署更新操作，适用于 CI/CD 流水线、自动化脚本等场景。

**基础 URL**: `http://<host>:<port><base_path>`

示例: `http://192.168.1.100:8080/api`

## 2. 认证

### 2.1 API Token 认证（推荐用于脚本/CI/CD）

在请求头中携带 Bearer Token：

```
Authorization: Bearer <api_token>
```

`api_token` 在服务端 `config.yaml` 的 `auth.api_token` 中配置。

### 2.2 Cookie 会话认证（用于浏览器）

先调用登录接口获取会话 cookie，后续请求自动携带。

### 2.3 认证失败

未认证或认证失败时返回：

```
HTTP/1.1 401 Unauthorized
Content-Type: text/plain; charset=utf-8

无效的 API Token
```

## 3. 接口列表

### 3.1 异步部署（一步到位）

**POST /api/deploy/upload**

接收 zip 文件和部署参数，异步执行部署流程，立即返回 `deploy_id`。

**请求**: `multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| file | file | 是 | zip 压缩包 |
| service | string | 是 | 目标服务名（需在 config.yaml services 中配置） |
| target_dir | string | 是 | 相对 root_dir 的目标目录 |
| files_only | string | 否 | "true"=仅替换文件不启停服务，默认 "false" |

**部署流程说明**：

默认部署流程按顺序执行三个步骤：

1. **stop** — 停止目标 Windows 服务（若已停止则跳过）
2. **deploy** — 解压 zip 并覆盖文件到目标目录
3. **start** — 启动目标 Windows 服务

当 `files_only=true` 时，**跳过步骤 1 和 3**，仅执行文件替换，不启停服务。适用于服务已手动停止或无需重启的场景。

**失败处理**：任一步骤失败立即中止后续步骤。停服务失败 → 不覆盖文件；覆盖失败 → 不启动服务；启动失败 → 文件已覆盖，无法自动回退。

**响应** (200):

```json
{
  "deploy_id": "1709644800000000",
  "status": "pending"
}
```

**curl 示例**:

```bash
curl -X POST http://192.168.1.100:8080/api/deploy/upload \
  -H "Authorization: Bearer my-secret-token" \
  -F "file=@update.zip" \
  -F "service=MyAppService" \
  -F "target_dir=app/target"
```

### 3.2 查询部署状态

**GET /api/deploy/status?id={deploy_id}**

查询异步部署的执行状态和步骤详情。

**参数**:

| 参数 | 位置 | 必填 | 说明 |
|------|------|------|------|
| id | query | 是 | 部署 ID（由 deploy/upload 返回） |

**响应** (200):

```json
{
  "deploy_id": "1709644800000000",
  "status": "success",
  "steps": [
    {"name": "stop", "status": "success", "message": "服务已停止", "duration": 3205},
    {"name": "deploy", "status": "success", "message": "已覆盖 5 个文件到 app/target", "duration": 128},
    {"name": "start", "status": "success", "message": "服务已启动", "duration": 1500}
  ],
  "final_state": "success",
  "service": "MyAppService",
  "target_dir": "app/target",
  "file_name": "update.zip",
  "created_at": "2026-08-04T10:00:00Z"
}
```

**status 取值**:

| 值 | 说明 |
|------|------|
| pending | 已接收，等待执行 |
| running | 正在执行 |
| success | 全部步骤成功 |
| failed | 某步骤失败 |

**curl 示例**:

```bash
curl http://192.168.1.100:8080/api/deploy/status?id=1709644800000000 \
  -H "Authorization: Bearer my-secret-token"
```

**轮询示例** (bash):

```bash
DEPLOY_ID=$(curl -s -X POST http://host:8080/api/deploy/upload \
  -H "Authorization: Bearer token" \
  -F "file=@update.zip" \
  -F "service=MyAppService" \
  -F "target_dir=app" | jq -r '.deploy_id')

while true; do
  STATUS=$(curl -s "http://host:8080/api/deploy/status?id=$DEPLOY_ID" \
    -H "Authorization: Bearer token" | jq -r '.status')
  echo "部署状态: $STATUS"
  if [ "$STATUS" = "success" ] || [ "$STATUS" = "failed" ]; then
    break
  fi
  sleep 2
done
```

### 3.3 上传文件

**POST /api/upload**

上传文件（支持 zip 和普通文件），返回 `upload_id` 用于后续部署。

**请求**: `multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| files | file[] | 是 | 一个或多个文件（也支持单字段 file） |

**响应** (200):

```json
{
  "upload_id": "1709644800000001",
  "files": [
    {"name": "update.zip", "size": 1024, "is_zip": true}
  ]
}
```

### 3.4 同步部署

**POST /api/deploy**

使用已上传的文件执行同步部署（等待完成）。

**请求**: `application/json`

```json
{
  "upload_id": "1709644800000001",
  "service": "MyAppService",
  "target_dir": "app/target",
  "files_only": false
}
```

**响应** (200):

```json
{
  "steps": [
    {"name": "stop", "status": "success", "message": "服务已停止", "duration": 3205},
    {"name": "deploy", "status": "success", "message": "已覆盖 5 个文件到 app/target", "duration": 128},
    {"name": "start", "status": "success", "message": "服务已启动", "duration": 1500}
  ],
  "final_state": "success"
}
```

### 3.5 服务列表

**GET /api/services**

**响应** (200):

```json
[
  {"name": "MyAppService", "display_name": "我的应用服务", "status": "running"}
]
```

### 3.6 服务控制

**POST /api/service/control**

**请求**: `application/x-www-form-urlencoded`

| 字段 | 说明 |
|------|------|
| service | 服务名 |
| action | start / stop / restart |

### 3.7 操作历史

**GET /api/history**

返回最近 50 条操作历史。

### 3.8 目标路径

**GET /api/targets?service={name}**

**POST /api/targets**

管理目标路径配置。

### 3.9 节点列表

**GET /api/nodes**

返回可参与批量更新的节点列表（未配置 `cluster` 时仅返回本机）。

**响应** (200):

```json
[
  {"name": "node-1", "url": "http://10.0.0.11:8080", "is_self": true},
  {"name": "node-2", "url": "http://10.0.0.12:8080", "is_self": false}
]
```

### 3.10 批量部署（多节点滚动更新）

**POST /api/cluster/deploy**

使用已上传的文件，在选中的多个节点上**串行滚动**执行部署（停服 → 覆盖 → 启服），立即返回 `batch_id`。

**请求**: `application/json`

```json
{
  "upload_id": "1709644800000001",
  "service": "MyAppService",
  "target_dir": "app/target",
  "files_only": false,
  "nodes": ["node-1", "node-2"]
}
```

- `nodes` 为空表示全部节点；执行顺序始终按 `cluster.nodes` 配置顺序。
- 同一时刻只有一个节点停机；单个节点失败不会中止后续节点。

**响应** (200):

```json
{"batch_id": "1709644800000099", "status": "pending"}
```

### 3.11 批量部署状态

**GET /api/cluster/status?id={batch_id}**

**响应** (200):

```json
{
  "batch_id": "1709644800000099",
  "status": "partial",
  "service": "MyAppService",
  "target_dir": "app/target",
  "file_name": "update.zip",
  "files_only": false,
  "nodes": [
    {
      "name": "node-1", "is_self": true, "status": "success", "final_state": "success", "duration": 5320,
      "steps": [{"name": "stop", "status": "success", "message": "服务已停止", "duration": 3205}]
    },
    {
      "name": "node-2", "is_self": false, "status": "failed", "final_state": "failed", "duration": 1200,
      "steps": [{"name": "remote", "status": "failed", "message": "分发文件到节点 node-2 失败: ...", "duration": 0}]
    }
  ],
  "created_at": "2026-10-30T10:00:00Z"
}
```

**status 取值**: `pending` / `running` / `success`（全部成功）/ `partial`（部分失败）/ `failed`（全部失败）

远端节点由当前节点调用其 `POST /api/upload` 与 `POST /api/deploy` 完成，因此每个节点都需要运行本工具，并在 `cluster.nodes` 中配置对方的 `url` 与 `api_token`。

## 4. 错误码

| HTTP 状态码 | 说明 |
|-------------|------|
| 200 | 成功 |
| 400 | 请求参数错误（缺少必填字段、zip 校验失败等） |
| 401 | 未认证（Token 无效或未登录） |
| 404 | 资源不存在（部署 ID 不存在等） |
| 405 | 方法不允许 |
| 429 | 请求过于频繁（触发限流） |

**429 响应头**:

```
Retry-After: 60
```

## 5. 限流

当 `rate_limit.enabled: true` 时，API 接口受请求频率限制：

- 默认每分钟 60 次请求
- `per_ip: true` 按客户端 IP 独立计数
- `per_ip: false` 全局共享计数
- 超限返回 429，响应头包含 `Retry-After`

## 6. 对接步骤

### 6.1 服务端配置

1. 在 `config.yaml` 中设置 `auth.api_token`（建议使用随机字符串）
2. 按需配置 `rate_limit`
3. 多节点部署时，每台服务器都需运行本工具，并配置 `node_name` 与 `cluster.nodes`（含对方 `url` 与 `api_token`）
4. 重启服务

### 6.2 客户端对接（curl）

```bash
# 1. 异步部署
RESPONSE=$(curl -s -X POST http://host:8080/api/deploy/upload \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@update.zip" \
  -F "service=MyAppService" \
  -F "target_dir=app")

DEPLOY_ID=$(echo $RESPONSE | jq -r '.deploy_id')
echo "部署已提交，ID: $DEPLOY_ID"

# 2. 查询状态
curl -s "http://host:8080/api/deploy/status?id=$DEPLOY_ID" \
  -H "Authorization: Bearer YOUR_TOKEN" | jq
```

### 6.3 CI/CD 集成示例（GitHub Actions）

```yaml
- name: 部署更新
  run: |
    DEPLOY_ID=$(curl -s -X POST ${{ secrets.DEPLOY_URL }}/api/deploy/upload \
      -H "Authorization: Bearer ${{ secrets.DEPLOY_TOKEN }}" \
      -F "file@update.zip" \
      -F "service=MyAppService" \
      -F "target_dir=app" | jq -r '.deploy_id')

    for i in $(seq 1 30); do
      STATUS=$(curl -s "${{ secrets.DEPLOY_URL }}/api/deploy/status?id=$DEPLOY_ID" \
        -H "Authorization: Bearer ${{ secrets.DEPLOY_TOKEN }}" | jq -r '.status')
      if [ "$STATUS" = "success" ]; then exit 0; fi
      if [ "$STATUS" = "failed" ]; then exit 1; fi
      sleep 2
    done
    echo "部署超时"
    exit 1
```

### 6.4 CI/CD 集成示例（PowerShell）

```powershell
# 异步部署
$response = Invoke-RestMethod -Uri "http://host:8080/api/deploy/upload" `
  -Method Post `
  -Headers @{ Authorization = "Bearer YOUR_TOKEN" } `
  -Form @{ file = Get-Item -Path "update.zip"; service = "MyAppService"; target_dir = "app" }

$deployId = $response.deploy_id
Write-Host "部署已提交，ID: $deployId"

# 轮询状态
do {
    Start-Sleep -Seconds 2
    $status = Invoke-RestMethod -Uri "http://host:8080/api/deploy/status?id=$deployId" `
      -Headers @{ Authorization = "Bearer YOUR_TOKEN" }
    Write-Host "部署状态: $($status.status)"
} while ($status.status -eq "pending" -or $status.status -eq "running")

if ($status.status -ne "success") {
    Write-Error "部署失败"
    exit 1
}
```

### 6.5 生成 API Token

建议使用以下方式生成随机 Token：

```bash
# Linux/Mac
openssl rand -hex 32

# PowerShell
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Maximum 256 }) -as [byte[]])
```
