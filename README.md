# Nginx Manager

基于 Go + Wails + Wails Desktop Kit 的多服务器 Nginx / OpenResty 管理工具。

## 架构

- `nginx-manager`：部署在 Linux 服务器上的 CLI / HTTP 管理端。
- `nginx-manager-desktop`：桌面控制台，可同时保存多个 CLI Endpoint。
- Desktop 普通连接信息使用 Kit `jsonstore`；管理密码使用 Kit `secureconfig`，不会明文进入 settings.json。
- CLI 只保存 bcrypt 密码哈希；未配置密码时拒绝启动管理 API。
- Linux 上 HTTP API 拒绝以 root 身份运行；写配置通过固定的 `privileged apply` 入口最小提权。
- 远程 Endpoint 必须使用 HTTPS；HTTP 仅允许 localhost / loopback，适合 SSH Tunnel。

## Release 资产

正式 tag（`v*`）会在同一个 GitHub Release 中发布：

- CLI：Windows amd64、Linux amd64、macOS amd64 / arm64；
- Desktop：Windows amd64、Linux amd64（raw / deb / tar.gz）、macOS universal；
- 每个二进制/安装包对应的 SHA256 校验文件。

CLI 资产名以 `nginx-manager-<tag>-...` 开头；Desktop 资产名以 `nginx-manager-desktop-<tag>-...` 开头。Release 构建会把 tag 注入 CLI 版本，`/api/v1/info` 不再显示 `dev`。

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
GET    /healthz
GET    /api/v1/info                      Basic Auth
GET    /api/v1/capabilities              Basic Auth
GET    /api/v1/diagnostics               Basic Auth
GET    /api/v1/deployment/plan            Basic Auth
GET    /api/v1/nginx/status              Basic Auth
GET    /api/v1/privilege/status          Basic Auth
GET    /api/v1/sites                     Basic Auth
GET    /api/v1/sites/{id}/config         Basic Auth
GET    /api/v1/sites/{id}/upstream-health Basic Auth
POST   /api/v1/sites/reverse-proxy       Basic Auth
PUT    /api/v1/sites/{id}/reverse-proxy   Basic Auth
PUT    /api/v1/sites/{id}/enabled         Basic Auth
PUT    /api/v1/sites/{id}/tls             Basic Auth
DELETE /api/v1/sites/{id}                Basic Auth
POST   /api/v1/nginx/reload               Basic Auth
GET    /api/v1/snapshots                 Basic Auth
POST   /api/v1/snapshots/{id}/restore    Basic Auth
GET    /api/v1/certificates              Basic Auth
POST   /api/v1/sites/{id}/certificate    Basic Auth
POST   /api/v1/certificates/renew        Basic Auth
GET    /api/v1/logs                      Basic Auth
GET    /api/v1/logs/{id}?lines=200       Basic Auth
```

Basic Auth 用户名固定为 `admin`，密码为 CLI 初始化时设置的管理密码。

`/api/v1/info` 与 `/api/v1/capabilities` 会返回 API 版本和能力列表。Desktop 只有在服务端明确声明能力时才按能力禁用页面；旧版 CLI 没有能力字段时进入兼容模式，不会把功能误判为不支持。这样同一个 Desktop 可以更安全地管理不同版本的 CLI。

`/api/v1/deployment/plan` 由 CLI 根据服务器真实状态生成生产部署步骤和命令，Desktop 只负责展示与复制，避免在不同 CLI 版本之间硬编码 sudoers、systemd 和路径模板。

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

Manager 站点在停用或删除前会自动保存 root-only 快照；`sites-enabled` 与 `conf.d` 两种常见布局都支持安全启停。快照默认保留最近 200 个；正式部署通过 root-owned `/etc/nginx-manager/paths.json` 的 `snapshot_retention` 调整。

反向代理还支持受控高级参数：最大请求体、上游连接超时和上游读取超时。参数只接受整数范围，不开放任意 Nginx 指令；站点编辑、HTTPS 开关和证书操作都会保留这些参数。

站点配置支持只读预览。Manager / 外部站点都可读取，但只允许站点目录内的常规配置文件，拒绝逃逸 symlink，单文件限制为 256 KiB。

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

HTTPS 可以事务关闭或切换 HTTP → HTTPS 跳转。关闭 HTTPS 时不会删除证书文件，并会保留 ACME Challenge 路由；如果服务器仍有匹配且有效的证书，之后可以不重新签发直接启用 HTTPS。

可选的 systemd renewal timer 每天检查两次 Certbot 续期，并加入随机延迟，续期完成后自动执行 `nginx -t` 和 reload。自动续期服务直接由 root systemd oneshot 执行，不加入 Desktop/API 使用的 sudoers 规则。

## 日志读取

日志读取同样走受限 root helper。Desktop 不发送日志路径，只发送由服务端生成的日志 ID。

Manager 新建或编辑的反向代理会自动使用站点独立日志：

```text
/var/log/nginx/nginx-manager/<域名>.access.log
/var/log/nginx/nginx-manager/<域名>.error.log
```

正式部署通过 root-owned `/etc/nginx-manager/paths.json` 的 `site_log_dir` 指定其他绝对目录；旧环境变量仅保留为开发/兼容 fallback。旧 Manager 站点会在下一次编辑时接入独立日志；外部配置不会自动改写。站点列表的“日志”快捷入口会优先直接打开独立 access log。

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

CLI 可以直接生成 sudoers、systemd 与可信路径配置模板：

```bash
nginx-manager privileged sudoers
nginx-manager service systemd
nginx-manager service renewal-service
nginx-manager service renewal-timer
nginx-manager config paths-file
nginx-manager config paths-template
nginx-manager doctor
nginx-manager doctor --json
```

`nginx-manager doctor` 提供服务用户视角的一次性只读诊断；正式部署建议使用 `sudo -u nginx-manager -H nginx-manager doctor`，确保密码配置目录与 systemd API 服务一致。

完整部署步骤见 `docs/deployment.md`。

## Desktop

```powershell
cd cmd/nginx-manager-desktop
wails dev
```

Desktop 当前能：

- 保存多个 CLI 连接；
- 对每个 CLI 进行 API 版本 / 能力协商；明确缺失能力时禁用对应页面，旧版 CLI 自动进入兼容模式；
- 切换当前连接，并一键检查全部 CLI 的可达性 / 管理权限状态；
- 安全保存每个连接的密码；
- 测试 CLI 认证与连通性；
- 自动聚合服务器总览：Nginx 配置、站点、HTTPS、证书、日志、自动续期和最近 access log 样本；
- 安全 Reload：先执行 `nginx -t`，只有配置通过才 reload；
- 展示服务器 Hostname、CLI 版本、Nginx/OpenResty Runtime；
- 独立检查受限 root helper / `nginx -t` 是否就绪；
- 读取当前服务器站点并区分 Manager 管理 / 外部配置；
- 只读预览 Manager / 外部站点的 Nginx 配置；
- 创建反向代理，并展示事务执行结果；
- 编辑 Manager 反向代理的域名、上游、WebSocket、请求体和超时设置；
- 检查单个或批量 Manager 反向代理上游健康状态；检查目标只来自已保存的 `proxy_pass`，不接受任意 URL；
- 启用、停用、删除 Manager 管理的站点；
- 查看最近的配置快照；
- 事务恢复历史快照，恢复前再次自动保存当前状态；
- 查看 Certbot / 证书状态与到期时间；
- 为 Manager 站点申请或更新 Let’s Encrypt 证书；
- 开启 / 关闭 HTTPS，复用已有证书重新启用，并切换 HTTP → HTTPS 强制跳转；
- 手动执行 Certbot 续期检查；
- 安全浏览 Nginx/OpenResty 访问日志与错误日志尾部内容，可选每 10 秒自动刷新；
- 在 Desktop 本地按站点域名和关键词过滤当前日志样本，并从站点列表快捷跳转到日志 / HTTPS 管理；
- 总览展示证书过期/临期、HTTPS 站点证书匹配、自动续期维护提醒和按域名匹配的站点访问样本；
- 支持 15 分钟 / 1 小时 / 6 小时 / 24 小时访问样本窗口（基于最近最多 1000 行，不作为完整历史统计）；
- 系统诊断页只读展示 nginx-manager.service、Nginx/OpenResty service、Certbot、renewal timer、运行时布局和可信路径配置；
- 部署向导按服务用户、密码归属、可信路径、最小 sudoers、systemd、Certbot/续期和 doctor 验证逐步检查，每一步只生成可复制命令，Desktop 不远程执行 root 安装操作。

下一阶段：更长期的流量统计与跨服务器批量运维。
