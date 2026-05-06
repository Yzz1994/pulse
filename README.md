# Pulse

代理节点控制面与节点管理系统。单仓双进制：`pulse-server`（控制面）和 `pulse-node`（节点面），通过 mTLS 通信。

## 功能特性

- **多节点管理** — 在控制面统一管理任意数量的代理节点，通过 RPC 热更新节点配置
- **多协议支持** — 基于 Xray-core，支持 VLESS、VLESS+Reality、Trojan、Shadowsocks、AnyTLS 等协议
- **用户与流量计费** — 流量配额、到期时间、状态机（active / disabled / limited / expired / on_hold）、按 inbound 维度设置流量倍率
- **订阅链接** — 自动生成 base64 订阅端点，兼容主流客户端
- **NodeGate（内置 SNI 代理）** — 无需 Nginx/Caddy，443 端口 TLS 终止、ACME 自动证书（支持 Cloudflare DNS-01）、HTTP 反向代理均在节点进程内完成
- **出口链路** — 在面板配置出站节点（Shadowsocks / VLESS+Reality），实现落地机流量转发
- **套餐与商店** — 套餐管理 + Stripe 付款，用户自助购买订阅
- **用户分组** — 将用户划分至不同分组，批量控制节点访问权限
- **IP Sentinel** — 基于 IP 的连接频率限制与访问控制
- **GeoIP 路由规则** — 内置 GeoIP 数据库，支持按国家/地区分流
- **审计日志** — 管理操作全程记录
- **工单系统** — 用户与管理员的消息通道

## 架构

```
pulse-server
  ├── REST API (/v1/*)              # 管理 API
  ├── 管理面板 (/panel/*)           # React SPA（内嵌于二进制）
  ├── 订阅端点 (/sub/:token)        # 返回 base64 代理链接列表
  ├── PostgreSQL 持久化
  └── 定时调度器（流量同步 / 重置 / on_hold 激活）

pulse-node
  ├── RPC API (/v1/node/*)          # 接受控制面指令
  ├── Xray 进程管理
  └── NodeGate（内置 SNI 代理 + ACME）
```

控制面与节点通过 **mTLS** 通信：控制面启动时自签客户端证书，安装节点时需将证书内容粘贴到节点机器。

## 开发

**依赖**：Go 1.21+、Bun、PostgreSQL

```bash
# 启动 server(:8080) + node(:8081) + React SPA dev server(:3000)
make dev

# 停止所有开发进程
make stop

# 运行测试
make test

# 编译所有二进制（同时构建并嵌入前端）
make build
```

默认地址：`http://localhost:3000`（面板），账号 `admin` / 密码 `admin123`。

> `make dev` 使用硬编码的开发凭据，不要用于生产环境。

## 生产安装

### 1. 安装 server

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

### 2. 获取节点证书

登录控制面 → **Settings** 页面，复制「Node 客户端证书」区块中的 PEM 内容。

### 3. 安装 node

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

### 4. 配置出口（可选）

1. 进入 **面板 → Outbounds**，添加出口（Shadowsocks 或 VLESS+Reality）
2. 在对应 inbound 编辑页的「出口」下拉框中选择出口，保存后下发配置即生效

不绑定出口的 inbound 保持直连。

### 5. 启用 NodeGate（可选，推荐）

NodeGate 是节点内置的 SNI 代理，无需安装 Nginx 或 Caddy。

1. 进入 **面板 → NodeGate**，编辑节点，填写 **ACME Email** 和 **面板域名**
2. 保存后面板自动触发同步，证书由 Let's Encrypt 自动申请

**无公网 80/443（NAT 机器）**：在 **面板 → 设置 → Cloudflare API Token** 配置 Token（需 `Zone:DNS:Edit` 权限），启用 DNS-01 验证。

详见 [docs/sniproxy.md](docs/sniproxy.md)。

### 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/uninstall.sh | sh
```

卸载脚本停止并删除 systemd 服务、所有二进制、配置文件及数据目录（`/var/lib/pulse`）。

## 发布新版本

交互式选择 patch / minor / major，先运行测试，通过后自动打 tag 并推送，触发 GitHub Actions 构建：

```bash
make release
```

## 许可证

[GNU Affero General Public License v3.0](LICENSE)
