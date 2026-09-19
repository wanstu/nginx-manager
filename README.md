# Nginx Manager

基于 Go + Wails + Wails Desktop Kit 的多服务器 Nginx / OpenResty 管理工具。

## 架构

- `nginx-manager`：部署在 Linux 服务器上的 CLI / HTTP 管理端。
- `nginx-manager-desktop`：桌面控制台，可同时保存多个 CLI Endpoint。
- Desktop 普通连接信息使用 Kit `jsonstore`；管理密码使用 Kit `secureconfig`，不会明文进入 settings.json。
- CLI 只保存 bcrypt 密码哈希；未配置密码时拒绝启动管理 API。
- Linux 上 HTTP API 拒绝以 root 身份运行；写配置通过固定的 `privileged apply` 入口最小提权。
- 远程 Endpoint 必须使用 HTTPS；HTTP 仅允许 localhost / loopback，适合 SSH Tunnel。

## CLI

首次必须设置密码：

```bash
nginx-manager auth set-password
```

启动：

```bash
nginx-manager serve --listen 127.0.0.1:8020
```

如果需要远程管理，建议仍让 CLI 监听本机，再通过 Nginx/Caddy HTTPS 反代；或者使用 SSH Tunnel。

当前 API：

```text
GET  /healthz
GET  /api/v1/info                 Basic Auth
GET    /api/v1/nginx/status              Basic Auth
GET    /api/v1/privilege/status          Basic Auth
GET    /api/v1/sites                     Basic Auth
POST   /api/v1/sites/reverse-proxy       Basic Auth
PUT    /api/v1/sites/{id}/reverse-proxy   Basic Auth
PUT    /api/v1/sites/{id}/enabled         Basic Auth
DELETE /api/v1/sites/{id}                Basic Auth
GET    /api/v1/snapshots                 Basic Auth
POST   /api/v1/snapshots/{id}/restore    Basic Auth
GET    /api/v1/certificates              Basic Auth
POST   /api/v1/sites/{id}/certificate    Basic Auth
POST   /api/v1/certificates/renew        Basic Auth
GET    /api/v1/logs                      Basic Auth
GET    /api/v1/logs/{id}?lines=200       Basic Auth
```

Basic Auth 用户名固定为 `admin`，密码为 CLI 初始化时设置的管理密码。

### 反向代理事务

新建反向代理不会直接“写文件然后 reload”，而是：

1. 校验 `server_name` 与 `proxy_pass` 输入；
2. 在临时目录生成候选站点和最小 Nginx 配置；
3. 对候选执行 `nginx -t`；
4. 候选通过后才写入真实站点目录并启用；
5. 对完整线上配置再次执行 `nginx -t`；
6. 完整测试通过后执行 reload；
7. 全局测试或 reload 失败时删除本次新增配置并恢复旧运行配置。

Manager 创建的站点带 `# managed-by: nginx-manager` 标记。现有外部配置可以读取，但不会被新建、启停或删除操作覆盖。

Manager 站点在停用或删除前会自动保存 root-only 快照；`sites-enabled` 与 `conf.d` 两种常见布局都支持安全启停。

## HTTPS / ACME

HTTPS 由 Nginx Manager 控制 Nginx 配置，Certbot 只负责签发/续期证书，不使用 `certbot --nginx` 修改站点文件。

首次签发流程：

1. 为 Manager 站点临时加入 `/.well-known/acme-challenge/` Webroot；
2. `nginx -t` 成功后 reload；
3. 使用固定参数执行 Certbot HTTP-01；
4. 验证签发证书与站点域名匹配；
5. 生成 443 TLS 配置，可选 HTTP → HTTPS 301；
6. 再次 `nginx -t` 后 reload；
7. 任一步失败都恢复签发前站点配置。

已启用 HTTPS 的站点编辑上游或 WebSocket 时会保留证书配置；不能直接把域名改成与现有证书不匹配的新域名。

## 日志读取

日志读取同样走受限 root helper。Desktop 不发送日志路径，只发送由服务端生成的日志 ID。

服务端会：

- 从 `nginx -T` 解析当前 `access_log` / `error_log`；
- 仅接受 Nginx/OpenResty 常见日志根目录中的路径；
- tail 时重新解析当前配置并按日志 ID 匹配；
- 拒绝 symlink 和非普通文件；
- 单次最多返回 1000 行 / 512 KiB。

## Linux 权限模型

`nginx-manager serve` 应以专用低权限用户运行。需要修改配置时，它只能执行 sudoers 明确放行的固定命令：

```text
/usr/local/bin/nginx-manager privileged apply
```

请求通过 stdin 使用结构化 JSON 协议传递，helper 只接受预定义操作，不提供 shell、命令路径或任意文件路径参数。

CLI 可以直接生成 sudoers 与 systemd 配置：

```bash
nginx-manager privileged sudoers
nginx-manager service systemd
```

完整部署步骤见 `docs/deployment.md`。

## Desktop

```powershell
cd cmd/nginx-manager-desktop
wails dev
```

Desktop 当前能：

- 保存多个 CLI 连接；
- 切换当前连接；
- 安全保存每个连接的密码；
- 测试 CLI 认证与连通性；
- 展示服务器 Hostname、CLI 版本、Nginx/OpenResty Runtime；
- 独立检查受限 root helper / `nginx -t` 是否就绪；
- 读取当前服务器站点并区分 Manager 管理 / 外部配置；
- 创建反向代理，并展示事务执行结果；
- 编辑 Manager 反向代理的域名、上游和 WebSocket 设置；
- 启用、停用、删除 Manager 管理的站点；
- 查看最近的配置快照；
- 事务恢复历史快照，恢复前再次自动保存当前状态；
- 查看 Certbot / 证书状态与到期时间；
- 为 Manager 站点申请或更新 Let’s Encrypt 证书；
- 可选 HTTP → HTTPS 强制跳转；
- 手动执行 Certbot 续期检查；
- 安全浏览 Nginx/OpenResty 访问日志与错误日志尾部内容。

下一阶段：自动续期 timer、证书自动维护、运行状态与流量概览。
