# 生产安装指南

Pulse 由两个二进制组成：

- **pulse-server** — 控制面（HTTP 面板 + 节点 gRPC Hub + 订阅服务）
- **pulse-node**   — 节点（管理 xray 进程，由控制面通过 gRPC 长连接下发指令）

后端持久化使用 **PostgreSQL**（必需）。

## 1. 安装 server

最简形式（脚本会自动检测并安装本机 PostgreSQL，使用本地 socket 创建数据库）：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) server
```

使用已有 PostgreSQL（强烈推荐用于生产）：

```bash
PULSE_DATABASE_URL='postgres://user:pass@host:5432/pulse?sslmode=disable' \
  bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) server
```

启用 Stripe 商店：

```bash
PULSE_STRIPE_SECRET_KEY='sk_live_xxx' \
  PULSE_STRIPE_WEBHOOK_SECRET='whsec_xxx' \
  bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) server
```

安装完成后会打印：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  面板地址: http://<IP>:<随机端口>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**首次访问面板会进入「初始化向导」**：在向导里设置第一个管理员用户名和密码（不通过环境变量）。

> 控制面只需开放一个端口（`PULSE_SERVER_ADDR`，默认随机端口）。面板 HTTP 与节点 gRPC
> 通过 cmux 共用该端口：Cloudflare 配置为 Flexible 模式（HTTP 到源站），节点直连走 TLS。

**server 安装脚本环境变量：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PULSE_DATABASE_URL` | 自动检测本机 PostgreSQL | PostgreSQL 连接串，格式 `postgres://user:pass@host:5432/db?sslmode=disable` |
| `PULSE_SERVER_ADDR` | 随机端口 | 监听地址（面板 + API + 订阅 + enroll + 节点 gRPC 共用），格式 `:端口` |
| `PULSE_NODE_GRPC_URL` | 脚本自动推断（`https://<公网IP>:<PULSE_SERVER_ADDR端口>`） | enroll 时返回给节点的 gRPC 拨号 URL；域名部署需手动设置 |
| `PULSE_NODE_CA_CERT_FILE` | `/etc/pulse/node_ca_cert.pem` | NodeCA 证书（首次启动自动生成） |
| `PULSE_NODE_CA_KEY_FILE` | `/etc/pulse/node_ca_key.pem` | NodeCA 私钥 |
| `PULSE_DATA_DIR` | `/var/lib/pulse` | 数据目录（geoip / 上传文件等） |
| `PULSE_INSTALL_BIN` | `/usr/local/bin` | 二进制安装目录 |
| `PULSE_INSTALL_ETC` | `/etc/pulse` | 配置目录 |
| `PULSE_STATE_DIR` | `/var/lib/pulse` | 工作目录 |
| `PULSE_STRIPE_SECRET_KEY` | — | Stripe Secret Key |
| `PULSE_STRIPE_WEBHOOK_SECRET` | — | Stripe Webhook Signing Secret |
| `PULSE_DISCOURSE_URL` | — | Discourse SSO 跳转地址，启用论坛单点登录 |
| `PULSE_DISCOURSE_SSO_SECRET` | — | Discourse SSO Secret |
| `PULSE_DISCOURSE_ADMIN_USERS` | — | Discourse 管理员用户名（逗号分隔） |
| `PULSE_DOWNLOAD_MIRROR` | — | GitHub 下载镜像前缀，例如 `https://ghfast.top/`（断网/慢网环境） |
| `PULSE_INSTALL_DRY_RUN` | — | 设为 `1` 时跳过下载、特权操作和数据库配置，仅 sanity check |

修改配置：

```bash
vim /etc/pulse/pulse-server.env
systemctl restart pulse-server
```

忘记管理员密码：登录数据库直接更新 `users` 表的 `password_hash` 字段，或在面板设置页内改密码。

## 2. 添加节点（生成安装命令）

登录控制面 → **节点**页面 → **添加节点** → 复制生成的完整安装命令（已包含
`--server`、`--node-id`、`--token` 三个参数）。token 是一次性的，一经使用即失效；
未使用且过期 24h 后由 `cleanup-enroll-tokens` 任务回收。

## 3. 安装 node

将上一步复制的命令粘贴到节点机器上执行，例如：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) node \
  --server https://<控制面板地址> \
  --node-id <节点ID> \
  --token <ENROLL_TOKEN>
```

也可以从 stdin 传入 token（避免 token 进入 shell history）：

```bash
echo "$ENROLL_TOKEN" | bash <(curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh) node \
  --server https://<控制面板地址> --node-id <节点ID> --token-file -
```

脚本会自动：

1. 下载并安装 `pulse-node` 二进制
2. 调用 `pulse-node enroll --server=<URL> --node-id=<ID> --token-file=<TMP> --insecure --out=/etc/pulse`
   向控制面 POST CSR，控制面用 NodeCA 签发节点证书，返回三件套
   `node_cert.pem` / `node_key.pem` / `node_ca.pem` 写入 `/etc/pulse/`
3. 把 `PULSE_NODE_ID` / `PULSE_NODE_GRPC_URL` / `PULSE_NODE_SERVER_ADDR` 等
   写入 `pulse-node.env`
4. 启动 systemd / OpenRC 服务

启动后 pulse-node 会主动连控制面 gRPC（与面板同端口，enroll 时写入 `PULSE_NODE_GRPC_URL`）建立长连接，**节点本身不监听任何端口**。

安装完成后显示：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  节点 ID:     <node-id>
  节点出口:    <node-ip>
  控制面 gRPC: https://<控制面板地址>:<面板端口>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**node 安装脚本环境变量：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PULSE_NODE_ID` | enroll 时传入 | 节点 ID（与证书 CN 一致），由脚本写入 env |
| `PULSE_NODE_GRPC_URL` | enroll 响应 | 控制面 gRPC URL，由脚本写入 env |
| `PULSE_NODE_SERVER_ADDR` | enroll 响应 | 控制面 gRPC `host:port`，由脚本写入 env |
| `PULSE_NODE_CLIENT_CERT_FILE` | `/etc/pulse/node_cert.pem` | 节点客户端证书（由 enroll 写入） |
| `PULSE_NODE_CLIENT_KEY_FILE` | `/etc/pulse/node_key.pem` | 节点客户端私钥（由 enroll 写入） |
| `PULSE_NODE_SERVER_CA_FILE` | `/etc/pulse/node_ca.pem` | 控制面 CA 证书（由 enroll 写入） |
| `PULSE_DOWNLOAD_MIRROR` | — | GitHub 下载镜像前缀 |
| `PULSE_INSTALL_DRY_RUN` | — | 设为 `1` 时跳过下载、特权操作和 enroll，仅 sanity check |

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

NodeGate **按需自动启停**：当节点上至少存在一条 host 把它当 relay 或一个 https_port>0 的入站时，控制面会自动下发 `Listen=":443"` 启动监听；否则保持关闭。

**无公网 80/443（NAT 机器）**：在 **面板 → 设置 → Cloudflare API Token** 配置 Token（需 `Zone:DNS:Edit` 权限），启用 DNS-01 验证。

详见 [docs/sniproxy.md](docs/sniproxy.md)。

## 6. 为面板启用 HTTPS（可选，推荐）

控制面板默认通过 `http://<server-ip>:8080/panel` 访问。最简单的方式是用 Cloudflare 反代 + 它自带的 SSL：

1. **DNS**：在 Cloudflare 添加 `panel.example.com` A 记录指向 server 的公网 IP，**开启橙云代理**（Proxied）。
2. **SSL/TLS → Overview**：模式选 **Flexible**（CF↔server 走 HTTP）。
   - 想要 CF↔server 也加密，可以在 server 前置一层 Caddy/Nginx 终止 TLS，再把 CF 模式调成 **Full** 或 **Full (strict)**。
3. **SSL/TLS → Edge Certificates**：打开 **Always Use HTTPS**，浏览器访问自动跳 HTTPS。
4. （可选）Origin Rules / Page Rules 把 `panel.example.com` → `:8080` 端口转发，避免在面板地址里带端口。

完成后 `https://panel.example.com/panel` 即可访问。订阅链接会自动用域名（panel 通过 `X-Forwarded-Host` 感知，无需改环境变量）。

> 不想用 Cloudflare：用 Caddy/Nginx 反代到 `http://127.0.0.1:8080`，或参考 [docs/sniproxy.md](docs/sniproxy.md) 把面板挂到节点 NodeGate 的 `http-reverse` 路由上。

## 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/uninstall.sh | sh
```

卸载脚本停止并删除 systemd 服务、所有二进制、配置文件及数据目录（`/var/lib/pulse`）。
PostgreSQL 中的 `pulse` 数据库**不会**自动删除，如需要请手动 `DROP DATABASE pulse`。
