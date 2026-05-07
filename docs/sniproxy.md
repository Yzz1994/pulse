# NodeGate (SNI Proxy)

pulse-node 内置的 Go 原生 SNI 代理，承担 443 端口的 TLS 终止、ACME 证书申请、SNI/HTTP 路由分发。

## 设计目标

- 证书申请、SNI 路由、TLS 终止都在 pulse-node 进程内完成，无需安装外部组件
- 和 xray 一样作为 Go 依赖嵌入，配置变更通过 pulse-server RPC 热更新

## 架构

```
┌─────────────┐
│  客户端      │ TLS (SNI=cdn-xxx.example.com)
└──────┬──────┘
       │
       ▼
┌────────────────────────────────────────────────────┐
│  pulse-node :443                                    │
│  ┌──────────────────────────────────────────────┐  │
│  │  sniproxy.UnifiedProxy                       │  │
│  │    1. peek TLS ClientHello 提取 SNI          │  │
│  │    2. 按 SNI 查路由表                        │  │
│  │    3. 按路由模式分发：                        │  │
│  │       - transparent → 原样 TCP 透传           │  │
│  │       - terminating → 终止 TLS → 明文 TCP     │  │
│  │       - http-reverse → 终止 TLS → HTTP 反代   │  │
│  └──────────────────────────────────────────────┘  │
│                                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │  certmgr.Manager                             │  │
│  │    certmagic + libdns/cloudflare              │  │
│  │    DNS-01 自动申请/续期 Let's Encrypt         │  │
│  │    证书持久化到 /var/lib/pulse-node/certs/    │  │
│  └──────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────┘
       │
       ▼
  本地 xray inbound (127.0.0.1:xxxxx)
  / 其他节点 (transparent 模式)
  / 本地 HTTP 服务 (http-reverse 模式)
```

## 三种路由模式

### `transparent`（透明转发）

前置节点用：不终止 TLS，读到 SNI 后原样 TCP 转发到落地节点。

```
Surge → cdn-ad5f.xxx.com (前置节点:443, SNI=cdn-ad5d.xxx.com)
       → sniproxy 读 SNI=cdn-ad5d → 透明 TCP 转发到落地 IP:443
                                    → 落地节点 sniproxy 再终止 TLS
```

不需要证书。落地节点需要有 SNI 对应的证书。

### `terminating`（TLS 终止 + TCP 明文转发）

落地节点给 xray 用：终止 TLS 后把明文字节转发给 xray 的 AnyTLS/Trojan inbound
（xray 监听 127.0.0.1，不暴露端口）。

```
客户端 → sniproxy :443 (SNI=cdn-ad5d) → 终止 TLS → 127.0.0.1:20149 (xray)
```

后端是裸二进制协议（AnyTLS/Trojan），sniproxy 不做任何 HTTP 层解析。
需要 certmgr 管理 SNI 对应的证书。

### `http-reverse`（HTTP 反向代理）

面板 / 用户自定义反代：终止 TLS 后作为 HTTP 服务器，请求通过
`httputil.ReverseProxy` 转发到本地 HTTP 后端，自动注入：
- `X-Forwarded-For`（保留上游链路）
- `X-Real-IP`
- `X-Forwarded-Proto: https`
- `X-Forwarded-Host`
- `Host` 改写为 SNI

自动处理：WebSocket `Upgrade`、HTTP/2 ↔ HTTP/1.1 翻译、SSE 流式无缓冲。

```
浏览器 → sniproxy :443 (SNI=panel.xxx.com) → 终止 TLS → HTTP → pulse-server:8080
```

## 配置

节点侧配置对象 `sniproxy.ManagerConfig`，由 pulse-server 通过 RPC 推送：

```json
{
  "listen": ":443",
  "cert_storage_path": "/var/lib/pulse-node/certs",
  "acme_email": "ops@example.com",
  "cloudflare_token": "cfut_xxx",
  "acme_staging": false,
  "routes": [
    {"sni": "cdn-ad5d.xxx.com", "backend": "127.0.0.1:20149", "mode": "terminating"},
    {"sni": "cdn-ad5f.xxx.com", "backend": "203.0.113.20:20148", "mode": "transparent"},
    {"sni": "panel.xxx.com",    "backend": "127.0.0.1:8080",   "mode": "http-reverse"}
  ]
}
```

持久化到节点 `{NodeTLSKeyFile 所在目录}/sniproxy_state.json`，重启自动恢复。

## pulse-server 端的生成逻辑

`jobs.BuildSNIProxySyncReq` 从数据库状态生成完整配置：

| 数据源 | 生成 |
|--------|------|
| 节点上的 `anytls`/`trojan` inbound + 各自 Host.SNI | `terminating` 路由 |
| 其他节点 Host 的 `relay_node_id` 指向本节点 | `transparent` 路由 |
| `nodes.panel_domain`（逗号/换行分隔多域名） | `http-reverse` → panel port |
| `nodes.extra_proxies`（每行 `domain:port`） | `http-reverse` → 127.0.0.1:port |
| 全局设置 `cf_token` | Cloudflare API token |
| `nodes.acme_email` | ACME 账户邮箱 |
| `nodes.https_port`（缺省则 host 级 `https_port`，再缺省 443） | listen 端口 |

## 触发点

每次下列事件 pulse-server 会向节点推送 NodeGate sync：

- `SyncUsage` 定时任务（每分钟）
- `triggerHostNodeSync`（Host 创建/更新后）
- `applyInboundNode` → `ApplyNode` → `ApplyNodeUsers`（Inbound 变更后）
- 外部模块手动触发

## 可视化和运维

### 面板

`/panel/sniproxy` 每节点一张卡片：
- 运行状态 badge（运行中 / 未接管 / 节点不可达）
- 最近错误 `last_error`
- 路由表（SNI / 模式 badge / Backend）
- 证书表（域名 / 就绪状态 / 到期日期 + 剩余天数 / 签发者）
- 证书存储路径
- 手动"同步"按钮

### 状态接口

- 节点侧：`GET /v1/node/sniproxy/status`（需 mTLS 客户端证书）
- 控制面代理：`GET /v1/nodes/{id}/sniproxy/status`

返回：
```json
{
  "enabled": true,
  "status": {
    "listen": ":443",
    "route_count": 5,
    "cert_domains": 3,
    "last_error": "",
    "routes": [...],
    "certs": [{"domain": "...", "not_after": "...", "ready": true, "issuer": "E8"}],
    "storage_path": "/var/lib/pulse-node/certs"
  },
  "config": {...}  // cloudflare_token 已脱敏
}
```

### 手动触发 sync

```bash
curl -X POST https://<server>:8080/v1/nodes/{nodeID}/sniproxy/sync \
  -H "Cookie: <登录 cookie>"
```

### 磁盘布局

```
{NodeTLSKeyFile 目录}/
├── server_cert.pem                # 节点 mTLS 服务端证书（xray 数据面）
├── server_key.pem
├── xray_last.json                 # xray 配置快照
└── sniproxy_state.json            # sniproxy Manager 持久化配置

/var/lib/pulse-node/certs/
└── certificates/
    └── acme-v02.api.letsencrypt.org-directory/
        └── <domain>/
            ├── <domain>.crt         # 可直接 openssl x509 查看
            ├── <domain>.key
            └── <domain>.json
```

## 功能矩阵

| 功能 | 实现 |
|------|------|
| AnyTLS SNI + TLS 终止 → xray | ✅ `terminating` |
| 前置节点 SNI 透传 | ✅ `transparent` |
| 面板 HTTPS 反代 | ✅ `http-reverse` |
| 用户 extra proxies | ✅ `http-reverse` |
| ACME DNS-01 Cloudflare | ✅ certmgr |
| 证书自动续期 | ✅ certmagic |
| Trojan over WebSocket `/ws` 反代 | ❌ 需要 xray 原生 TLS+WS |
| 非 443 端口的独立端口转发 | ❌ UnifiedProxy 只监听一个端口 |

## 已知限制 / 待改进

1. **Trojan+WS** 业务场景：NodeGate 目前不支持 HTTP path 匹配。替代方案：
   让 xray Trojan inbound 自己跑 TLS+WS（xray 原生支持），完全绕开 NodeGate。

2. **单端口限制 / 多端口 portforward**：顶层 `Listen` 全局只有一个，`UnifiedProxy`
   实例也只有一个，一个进程仅支持监听单端口。

   架构演进方向（未来有真实需求时一次性做）：把 `Listen` 字段下沉到 `Route`，
   `ManagerConfig.Listen` 作为默认值；Manager 按 `Route.Listen` 分组，每组起一个
   `UnifiedProxy`：

   ```json
   {
     "listen": ":443",           // 默认
     "routes": [
       {"sni": "cdn-a", "backend": "...", "mode": "terminating"},  // 用默认
       {"sni": "...",   "backend": "...", "mode": "transparent", "listen": ":41759"}  // 覆盖
     ]
   }
   ```

   这样非 443 portforward 天然变成一条普通路由，不需要单独的 TCPForwarder。

3. **SNI 大小写敏感**：`UnifiedProxy.SetRoutes` 按 SNI 原样建 map，客户端发 `Example.COM`
   不会匹配 `example.com`。实际场景里 CA 签证书都是小写域名，客户端按证书回填 SNI，
   通常不是问题。如需严格一致可在 `SetRoutes` 和 `handle` 的 lookup 都做
   `strings.ToLower`。

4. **Cloudflare token 明文落盘**：`sniproxy_state.json` 文件权限 `0600`，但 token
   是明文。重装机器后需重新下发（pulse-server 会自动在下次 sync 时补）。

## 开发者速查

```
internal/sniproxy/
├── peek.go           # TLS ClientHello 解析 → 提取 SNI
├── transparent.go    # TransparentProxy（独立实例；Manager 不用）
├── terminating.go    # TerminatingProxy（独立实例；Manager 不用）
├── unified.go        # UnifiedProxy（Manager 使用的主类型，三种模式）
└── manager.go        # Manager：lifecycle + 持久化 + certmgr wiring

internal/certmgr/
└── certmgr.go        # certmagic + Cloudflare DNS-01 的薄封装

internal/nodeapi/sniproxy.go        # 节点侧 RPC：/sync + /status
internal/serverapi/sniproxy.go      # 控制面：doSyncNodeSNIProxy / handleNodeSNIProxyStatus
internal/jobs/sniproxy.go           # 核心 builder：BuildSNIProxySyncReq

web/panel/src/routes/sniproxy.tsx   # 面板可视化页
```

## 相关 commit

- `13ec342` peek + transparent
- `d8a5544` terminating + certmgr
- `09eccb5` UnifiedProxy + Manager + pulse-node 集成
- `200302d` 自动触发 NodeGate sync
- `659cd75` P0 修复（Mode 字符串化、TLSConfig 竞争、失败探活、错误暴露）
- `57de8b3` 巩固修复（p.ln atomic、测试加 assert）
- `438ae4e` 面板可视化页
- `67dd7fb` http-reverse 模式（面板反代 + extra proxies）
