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
GET  /healthz
GET  /api/v1/info                 Basic Auth
GET  /api/v1/nginx/status         Basic Auth
GET  /api/v1/sites                Basic Auth
POST /api/v1/sites/reverse-proxy  Basic Auth
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

Manager 创建的站点带 `# managed-by: nginx-manager` 标记。现有外部配置可以读取，但不会被新建操作覆盖。

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
- 读取当前服务器站点并区分 Manager 管理 / 外部配置；
- 创建反向代理，并展示事务执行结果。

下一阶段：编辑/停用 Manager 站点、配置快照历史、证书 / ACME、访问日志与错误日志。
