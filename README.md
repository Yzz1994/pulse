# pulse

代理节点控制面与节点管理系统，控制面与节点统一在单一仓库。

## 目录结构

```text
.
├── cmd/pulse-server          # 控制面入口
├── cmd/pulse-node            # 节点入口
├── cmd/pulse                 # 合并入口（server + node）
├── internal/alert            # 告警通知（Bark）
├── internal/announcements    # 公告管理
├── internal/app              # 应用启动与生命周期
├── internal/auditlog         # 操作审计日志
├── internal/auth             # 管理员认证（含 Discourse SSO）
├── internal/backup           # 数据库备份（Cloudflare R2）
├── internal/buildinfo        # 版本信息
├── internal/cert             # 自签证书生成
├── internal/certmgr          # mTLS 证书管理
├── internal/cfdomain         # Cloudflare 域名管理
├── internal/cloudflarex      # Cloudflare API 客户端
├── internal/config           # 配置结构
├── internal/coremanager      # 代理核心（sing-box / xray）统一运行时接口
├── internal/geoip            # GeoIP 查询
├── internal/idgen            # Snowflake ID 生成
├── internal/inbounds         # inbound / host 模型与 store
├── internal/ipsentinel       # IP 哨兵（按来源 IP 的区域限流与封锁）
├── internal/jobs             # 后台调度任务（流量同步、重置、激活）
├── internal/node             # 节点侧服务
├── internal/nodeapi          # 节点 RPC API（含 NodeGate 同步、路由追踪）
├── internal/nodeauth         # 节点认证中间件
├── internal/nodes            # 节点 store 与 RPC client
├── internal/orders           # 订单管理
├── internal/outbounds        # outbound 出口模型与 store
├── internal/panel            # 公开 API（/v1/stat、用户订阅 Token 重置）
├── internal/payment          # 支付集成（Stripe）
├── internal/plans            # 套餐管理
├── internal/proxycfg         # 代理核心配置生成（sing-box / xray）
├── internal/routerules       # 路由规则管理
├── internal/server           # 控制面 HTTP 服务
├── internal/serverapi        # 控制面 REST API（含用户门户）
├── internal/singbox          # sing-box 进程管理
├── internal/xray             # xray-core in-process 生命周期管理与流量采集
├── internal/xrayanytls       # AnyTLS 入站服务（与 xray 并行，支持热更新用户）
├── internal/spa              # React SPA 嵌入服务
├── internal/store/postgres   # PostgreSQL 持久化
├── internal/subscription     # 订阅 URL 生成
├── internal/syslog           # 系统日志 SSE 推送
├── internal/tickets          # 用户工单系统
├── internal/usage            # 节点流量统计汇总
├── internal/users            # 用户模型与 store
├── web/panel/                # React SPA 前端（Bun + React 19 + TanStack Router + shadcn/ui）
├── scripts/install.sh          # 生产安装脚本
└── scripts/uninstall.sh        # 卸载脚本
```

## 开发

启动 server + node + React SPA 前端（开发模式，Bun dev server 监听 `:3000`，代理 API 到 `:8080`）：

```bash
make dev
```

停止开发进程：

```bash
make stop
```

运行测试：

```bash
make test
```

编译所有二进制（pulse / pulse-server / pulse-node），自动构建 React SPA 并嵌入 Go 二进制：

```bash
make build
```

默认访问地址：`http://localhost:3000`（面板），API 在 `:8080`，账号 `admin` / 密码 `admin123`，node 监听 `:8081`。

> `make dev` 使用硬编码的开发凭据，生产环境请使用安装脚本。

## 发布新版本

交互式选择 patch / minor / major，先运行测试，通过后自动打 tag 并推送，GitHub Actions 触发构建：

```bash
make release
```

## 生产安装

### 1. 安装 server

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server
```

**指定管理员密码：**

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | PULSE_ADMIN_PASSWORD='strong-password' sh -s -- server
```

**重置管理员密码（生成随机新密码并重启服务）：**

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server --reset-password
```

首次安装会随机生成端口和管理员密码，安装结束后打印：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  面板地址: http://<IP>:<随机端口>
  管理员:   admin
  密码:     <随机生成>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

如需修改端口或其他配置：

```bash
vim /etc/pulse/pulse-server.env
```

```bash
systemctl restart pulse-server
```

**启用 Stripe 商店：**

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | \
  PULSE_STRIPE_SECRET_KEY='sk_live_xxx' \
  PULSE_STRIPE_WEBHOOK_SECRET='whsec_xxx' \
  sh -s -- server
```

**安装脚本支持的环境变量（server）：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PULSE_ADMIN_USERNAME` | `admin` | 管理员用户名 |
| `PULSE_ADMIN_PASSWORD` | 随机生成 | 管理员密码 |
| `PULSE_SERVER_ADDR` | 随机端口 | server 监听地址，格式 `:端口` |
| `PULSE_INSTALL_BIN` | `/usr/local/bin` | 二进制安装目录 |
| `PULSE_INSTALL_ETC` | `/etc/pulse` | 配置安装目录 |
| `PULSE_STATE_DIR` | `/var/lib/pulse` | 数据目录 |
| `PULSE_STRIPE_SECRET_KEY` | — | Stripe Secret Key（`sk_live_xxx`），配置后自动启用商店 |
| `PULSE_STRIPE_WEBHOOK_SECRET` | — | Stripe Webhook Signing Secret（`whsec_xxx`） |

### 2. 获取 node 所需证书

登录控制面 → Settings 页面，复制「Node 客户端证书」区块中的 PEM 内容。

### 3. 在 node 机器上安装 node

安装最新版：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- node
```

指定 node 监听端口（默认 8081）：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | PULSE_NODE_PORT='9090' sh -s -- node
```

重新粘贴证书（已有证书需强制覆盖时）：

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- --force node
```

执行后脚本会提示粘贴证书，把第 2 步复制的 PEM 内容粘贴进去，输入空行确认后自动继续。安装完成后显示节点监听地址：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  节点地址: https://<IP>:<端口>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

如需修改端口或其他配置：

```bash
vim /etc/pulse/pulse-node.env
```

```bash
systemctl restart pulse-node
```

**安装脚本支持的环境变量（node）：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PULSE_NODE_ADDR` | `:8081` | node 监听地址，格式 `:端口` |
| `PULSE_NODE_PORT` | `8081` | node 监听端口（纯数字，优先于 ADDR） |
| `PULSE_NODE_TLS_CERT_FILE` | — | node 服务端证书路径 |
| `PULSE_NODE_TLS_KEY_FILE` | — | node 服务端私钥路径 |

### 4. 配置出口转发（可选）

默认所有流量直连。如需将某个 inbound 的流量转发到另一台落地机，可在面板配置出口：

1. 进入 **面板 → Outbounds**，点击「添加出口」
2. 选择协议（Shadowsocks 或 VLESS + Reality），填写落地机地址和认证信息
3. 进入对应 inbound 的编辑页，在「出口」下拉框中选择刚创建的出口，保存后应用节点配置即生效

不绑定出口的 inbound 保持直连。

### 5. 启用 HTTPS / NodeGate（可选，推荐生产环境）

pulse-node 内置 NodeGate（Go 原生 SNI 代理），无需额外安装外部组件。443 端口的 TLS 终止、ACME 证书申请、HTTP 反向代理由 NodeGate 自动完成。

**在面板配置：**

1. 进入 **面板 → NodeGate**，找到对应节点
2. 编辑节点，填写 **ACME Email**（Let's Encrypt 邮箱）和 **面板域名**（用于面板 HTTPS）
3. 保存后面板自动触发 NodeGate 同步

后续每次应用节点配置（新增/修改 inbound）时，面板会自动重新下发 NodeGate 路由表。

**NAT / 无公网 80/443 机器**：在 **面板 → 设置 → Cloudflare API Token** 配置 Token（需 `Zone:DNS:Edit` 权限），即可走 DNS-01 申请证书。

---

### 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/uninstall.sh | sh
```

卸载脚本会：停止并禁用 pulse-server、pulse-node systemd 服务，删除所有二进制、配置文件、服务文件，以及数据目录（`/var/lib/pulse`）。

---

### 安装脚本做了什么

- 从 GitHub Release 下载对应平台（linux/amd64 或 linux/arm64）的 tar.gz
- 安装二进制到 `/usr/local/bin`
- 首次安装时写入示例配置到 `/etc/pulse/*.env`（已有配置不覆盖）
- server：随机生成端口和管理员密码（可通过环境变量覆盖）；`--reset-password` 参数可强制重置密码
- node：交互式提示粘贴 server 客户端证书 PEM，写入 `/etc/pulse/server_client_cert.pem`
- 注册并启动 systemd 服务（`systemctl enable --now`）
