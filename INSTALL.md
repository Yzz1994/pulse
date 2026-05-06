# 生产安装指南

## 1. 安装 server

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server
```

指定管理员密码：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | PULSE_ADMIN_PASSWORD='strong-password' sh -s -- server
```

重置管理员密码：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server --reset-password
```

安装完成后打印：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  面板地址: http://<IP>:<随机端口>
  管理员:   admin
  密码:     <随机生成>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

启用 Stripe 商店：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | \
  PULSE_STRIPE_SECRET_KEY='sk_live_xxx' \
  PULSE_STRIPE_WEBHOOK_SECRET='whsec_xxx' \
  sh -s -- server
```

**server 安装脚本环境变量：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PULSE_ADMIN_USERNAME` | `admin` | 管理员用户名 |
| `PULSE_ADMIN_PASSWORD` | 随机生成 | 管理员密码 |
| `PULSE_SERVER_ADDR` | 随机端口 | 监听地址，格式 `:端口` |
| `PULSE_INSTALL_BIN` | `/usr/local/bin` | 二进制安装目录 |
| `PULSE_INSTALL_ETC` | `/etc/pulse` | 配置目录 |
| `PULSE_STATE_DIR` | `/var/lib/pulse` | 数据目录 |
| `PULSE_STRIPE_SECRET_KEY` | — | Stripe Secret Key |
| `PULSE_STRIPE_WEBHOOK_SECRET` | — | Stripe Webhook Signing Secret |

修改配置：

```bash
vim /etc/pulse/pulse-server.env
systemctl restart pulse-server
```

## 2. 获取节点证书

登录控制面 → **Settings** 页面，复制「Node 客户端证书」区块中的 PEM 内容。

## 3. 安装 node

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- node
```

指定监听端口（默认 8081）：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | PULSE_NODE_PORT='9090' sh -s -- node
```

执行后脚本提示粘贴证书（第 2 步复制的 PEM），输入空行确认，安装完成后显示：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  节点地址: https://<IP>:<端口>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**node 安装脚本环境变量：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PULSE_NODE_ADDR` | `:8081` | 监听地址，格式 `:端口` |
| `PULSE_NODE_PORT` | `8081` | 监听端口（优先于 ADDR） |
| `PULSE_NODE_TLS_CERT_FILE` | — | 自定义 TLS 证书路径 |
| `PULSE_NODE_TLS_KEY_FILE` | — | 自定义 TLS 私钥路径 |

修改配置：

```bash
vim /etc/pulse/pulse-node.env
systemctl restart pulse-node
```

## 4. 配置出口（可选）

1. 进入 **面板 → Outbounds**，添加出口（Shadowsocks 或 VLESS+Reality）
2. 在对应 inbound 编辑页的「出口」下拉框中选择出口，保存后下发配置即生效

不绑定出口的 inbound 保持直连。

## 5. 启用 NodeGate（可选，推荐）

NodeGate 是节点内置的 SNI 代理，无需安装 Nginx 或 Caddy。

1. 进入 **面板 → NodeGate**，编辑节点，填写 **ACME Email** 和 **面板域名**
2. 保存后面板自动触发同步，证书由 Let's Encrypt 自动申请

**无公网 80/443（NAT 机器）**：在 **面板 → 设置 → Cloudflare API Token** 配置 Token（需 `Zone:DNS:Edit` 权限），启用 DNS-01 验证。

详见 [docs/sniproxy.md](docs/sniproxy.md)。

## 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/uninstall.sh | sh
```

卸载脚本停止并删除 systemd 服务、所有二进制、配置文件及数据目录（`/var/lib/pulse`）。
