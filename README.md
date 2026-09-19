# Nginx Manager

基于 Go + Wails + Wails Desktop Kit 的多服务器 Nginx / OpenResty 管理工具。

## 架构

- `nginx-manager`：部署在 Linux 服务器上的 CLI / HTTP 管理端。
- `nginx-manager-desktop`：桌面控制台，可同时保存多个 CLI Endpoint。
- Desktop 普通连接信息使用 Kit `jsonstore`；管理密码使用 Kit `secureconfig`，不会明文进入 settings.json。
- CLI 只保存 bcrypt 密码哈希；未配置密码时拒绝启动管理 API。
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
GET /healthz
GET /api/v1/info          Basic Auth
GET /api/v1/nginx/status  Basic Auth
```

Basic Auth 用户名固定为 `admin`，密码为 CLI 初始化时设置的管理密码。

## Desktop

```powershell
cd cmd/nginx-manager-desktop
wails dev
```

Desktop v0.1 能：

- 保存多个 CLI 连接；
- 切换当前连接；
- 安全保存每个连接的密码；
- 测试 CLI 认证与连通性；
- 展示服务器 Hostname、CLI 版本、Nginx/OpenResty Runtime。

下一阶段：站点、反向代理、证书、配置事务、`nginx -t`、reload、快照与回滚。
