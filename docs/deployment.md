# Linux 部署

推荐模型：

- `nginx-manager serve` 以专用低权限用户运行；
- HTTP API 只监听 `127.0.0.1`；
- Desktop 通过 SSH Tunnel 或 HTTPS 反向代理访问；
- 需要修改 Nginx 时，API 只可通过固定命令 `nginx-manager privileged apply` 提权；
- sudoers 不允许任意参数、任意 shell 或任意文件写入。

## 1. 安装二进制

```bash
sudo install -m 0755 nginx-manager-linux-amd64 /usr/local/bin/nginx-manager
```

创建专用系统用户：

```bash
sudo useradd \
  --system \
  --user-group \
  --home-dir /var/lib/nginx-manager \
  --create-home \
  --shell /usr/sbin/nologin \
  nginx-manager
```

如果用户已经存在，可跳过。

## 2. 设置管理密码

必须以服务用户执行，确保密码配置写入服务用户自己的配置目录：

```bash
sudo -u nginx-manager -H /usr/local/bin/nginx-manager auth set-password
```

服务端只保存 bcrypt hash。

## 3. 安装最小 sudoers 规则

先生成：

```bash
/usr/local/bin/nginx-manager privileged sudoers \
  --service-user nginx-manager \
  --helper /usr/local/bin/nginx-manager \
  | sudo tee /tmp/nginx-manager.sudoers
```

验证：

```bash
sudo visudo -cf /tmp/nginx-manager.sudoers
```

验证通过后安装：

```bash
sudo install \
  -o root -g root -m 0440 \
  /tmp/nginx-manager.sudoers \
  /etc/sudoers.d/nginx-manager

sudo rm -f /tmp/nginx-manager.sudoers
```

生成的规则只允许：

```text
/usr/local/bin/nginx-manager privileged apply
```

不能借此运行任意 shell 或其他命令。

## 4. 安装 systemd 服务

生成 unit：

```bash
/usr/local/bin/nginx-manager service systemd \
  --service-user nginx-manager \
  --binary /usr/local/bin/nginx-manager \
  --listen 127.0.0.1:8020 \
  | sudo tee /etc/systemd/system/nginx-manager.service
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nginx-manager
sudo systemctl status nginx-manager --no-pager
```

检查监听：

```bash
sudo ss -lntp | grep 8020
```

应只看到：

```text
127.0.0.1:8020
```

## 5. 验证

健康检查不需要密码：

```bash
curl http://127.0.0.1:8020/healthz
```

验证认证：

```bash
curl -u admin http://127.0.0.1:8020/api/v1/info
```

验证受限 helper：

```bash
curl -u admin http://127.0.0.1:8020/api/v1/privilege/status
```

正常时应看到 `ready: true`；同时会返回 Nginx/OpenResty 运行时、配置布局和 `nginx -t` 状态。

## 6. Desktop 连接

推荐优先使用 SSH Tunnel，不开放 8020：

Windows PowerShell：

```powershell
ssh -N -L 8020:127.0.0.1:8020 admin@服务器IP
```

Desktop 新增连接：

```text
名称：阿里云 Debian
CLI Endpoint：http://127.0.0.1:8020
管理密码：刚才设置的密码
```

Desktop 允许回环地址使用 HTTP；非回环远程地址必须使用 HTTPS。

## 快照

Manager 管理的站点在停用或删除前会自动保存 root-only 快照：

```text
/var/lib/nginx-manager/snapshots
```

快照文件模式为 `0600`。外部手写配置没有 Manager 标记时不会被启停或删除。


## HTTPS / Certbot

服务器需要预先安装 Certbot。Debian 可使用系统包管理器安装，例如：

```bash
sudo apt update
sudo apt install -y certbot
```

签发前请确认：

- 域名 DNS 已解析到当前服务器；
- 公网 TCP 80 可访问当前 Nginx；
- 启用 HTTPS 后公网 TCP 443 可访问；
- 目标站点由 Nginx Manager 管理且处于启用状态。

Nginx Manager 使用 HTTP-01 Webroot：

```text
/var/lib/nginx-manager/acme-webroot
```

Certbot 只负责写入 Let’s Encrypt 证书；Nginx Manager 自己生成并事务应用 TLS 配置，不允许 Certbot 直接编辑站点配置。

证书元数据默认从以下目录读取：

```text
/etc/letsencrypt/live
```

可以在 Desktop 的 “HTTPS / 证书” 页面查看证书到期时间、申请证书、启用 HTTP → HTTPS 以及手动执行续期检查。


### 自动续期 timer

生成 root-only oneshot 服务：

```bash
/usr/local/bin/nginx-manager service renewal-service \
  --binary /usr/local/bin/nginx-manager \
  | sudo tee /etc/systemd/system/nginx-manager-renew.service
```

生成 timer：

```bash
/usr/local/bin/nginx-manager service renewal-timer \
  | sudo tee /etc/systemd/system/nginx-manager-renew.timer
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nginx-manager-renew.timer
sudo systemctl list-timers nginx-manager-renew.timer --no-pager
```

timer 每天在 03:00 和 15:00 两个窗口触发，并加入最多 45 分钟随机延迟。实际执行：

```text
/usr/local/bin/nginx-manager privileged renew-certificates
```

该命令要求 root；它**不会**加入 `/etc/sudoers.d/nginx-manager`，因此 Desktop/API 的低权限服务不能绕过结构化 `privileged apply` 协议直接调用它。
