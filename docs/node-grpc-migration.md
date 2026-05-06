# Pulse Node 通信层迁移规划

> 目标：将 server 主动 HTTP 调 node 的模型，改为 node 主动 gRPC 长连接 server，消除 node 对外监听端口的依赖。

---

## 背景

### 现状（Pull 模型）

```
pulse-server ──HTTP/mTLS──> pulse-node:8081
```

- Server 每分钟主动拉取流量、推送配置
- Node 必须对外监听 8081 端口（mTLS）
- Node 需要公网 IP，控制面必须能路由到节点

### 目标（Push 模型）

```
pulse-node ──gRPC/mTLS──> pulse-server:8082
```

- Node 主动连 server，保持 gRPC 双向流长连接
- Server 通过流发送指令，Node 返回响应
- Node 无需对外监听端口，可在 NAT 后运行

---

## 技术选型

### 协议：gRPC

选 gRPC，但需要诚实承认：**当前阶段 gRPC 相对 WebSocket+JSON 的收益并不明显，主要押注的是长期扩展性**。

| 维度 | gRPC | WebSocket + JSON | 说明 |
|------|------|-----------------|------|
| 双向流传输框架 | HTTP/2 多路复用 | 单 TCP 帧序列 | 单一双向流场景下两者都够用 |
| 请求-响应关联 | 仍需手写 pending map | 需手写 pending map | **gRPC 不会自动帮你做 id 对账**，stream RPC 同样要管 |
| 类型安全 | proto 编译期 | 运行时 | 本方案 body 暂用 JSON+`bytes`，**编译期收益被抵消**，等后续逐步迁移到 proto 类型才能兑现 |
| 超时/取消传播 | context deadline 走 HTTP/2 trailer | 需自定义 cancel 帧 | gRPC 略胜 |
| 流控/背压 | HTTP/2 内置 | 需自实现 | 高频日志/速率限制场景 gRPC 省事 |
| 工具链成本 | 引入 `protoc` + 代码生成 | 仅需第三方 ws 库 | gRPC 多一套生成流程，与现有 sqlc 并列 |
| 二进制增量 | ~5 MB | <1 MB | gRPC 略大 |
| 部署约束 | HTTP/2 + ALPN，**多数 CDN/L7 不透传**，需直连域名/端口 | 标准 HTTPS Upgrade，CDN 兼容性好 | 见"部署约束"小节 |
| 后续 proto 化迁移 | 平滑（同一连接换 message 类型） | 需重写 | 长期收益 |
| 生态 | `google.golang.org/grpc` 成熟 | `nhooyr.io/websocket` / `gorilla/websocket` 成熟 | 都不是问题 |

**结论**：选 gRPC 主要为了 (1) 后续把 body 从 JSON 迁移到 proto 类型 (2) 流控/取消语义 (3) relay 已有相同模式可参考。如果短期不打算做 proto 化、且部署在 CDN 后，WebSocket+JSON 是更轻的选择，可作为备选。

### 认证：mTLS（优于 Bearer Token）

| 维度 | mTLS | Bearer Token |
|------|------|-------------|
| 强度 | 双端密码学身份验证 | 单向，token 泄漏即失陷 |
| 轮换 | 证书有效期自动管理 | 需实现 token 轮换机制 |
| 基础设施 | pulse 已有自签证书逻辑 | 需新增 token 存储和验证 |
| 行业实践 | 服务间通信标准方案 | 适合用户侧 API |

---

## mTLS 证书方案

### 部署约束（重要）

gRPC 走 HTTP/2 + ALPN，**多数 CDN/L7 网关不会透传**（Cloudflare、AWS ALB Classic 等会终止 HTTP/2 并把 trailer 丢掉）。

因此：
- gRPC 端口（`:8082`）必须**直连 server**，不能走 CDN
- 如果 server 现在已经在 CDN 后，需要为 gRPC 单独开一个域名（如 `node.<domain>`）指向源站，或单独占用端口
- 节点端的 `enroll_token` 与 `node_grpc_url` 必须配套生成，避免管理员手动拼错

### Server 侧

Server 启动时生成一个 **Node CA**（独立于现有 server_client_cert）：

```
pulse-server
  └── node_ca_cert.pem   # CA 证书，用于签发 node 客户端证书
  └── node_ca_key.pem    # CA 私钥
```

gRPC 端口（`:8082`）的 TLS 配置：
- 服务端证书：复用现有 HTTPS 证书，或单独自签
- 客户端验证：`RequireAndVerifyClientCert`，信任 `node_ca_cert.pem`

### Node 侧（Enrollment 流程）

安装时执行一次性 enrollment，封装为 `pulse-node enroll` 子命令（不在 shell 里用 openssl 生成 CSR，统一在 Go 里做错误处理）：

```
$ pulse-node enroll --server=https://node.example.com:8082 --token=<one_time_token>

1. Node 本地生成 RSA 私钥 + CSR（in-process）
2. POST /v1/node-enroll  { node_id, csr_pem, enroll_token }
3. Server 验证 enroll_token（DB 查询 + 状态机），用 Node CA 签发证书，返回 cert_pem + ca_pem + node_grpc_url
4. Node 保存到 ./node_cert.pem / node_key.pem / node_ca.pem
5. install.sh 仅负责调用此子命令，不再粘贴证书
```

`enroll_token` 由控制面生成安装命令时一并签发：

| 字段 | 类型 | 说明 |
|------|------|------|
| `token` | text PK | 随机 32 字节 hex |
| `node_id` | text | 预绑定的 node_id（防止 token 被挪用到别的节点） |
| `expires_at` | timestamptz | 默认 1 小时 |
| `consumed_at` | timestamptz nullable | 使用后置时间，标记失效 |

**token 下发链路**：管理员在面板"添加节点"页面生成完整的 `pulse-node enroll --server=... --token=...` 命令，复制到节点机器执行；token 不进入 install.sh URL 参数（避免 shell history 泄漏），从 stdin 或 `--token-file` 读取。

定时任务清理 `expires_at < now() - 24h` 的记录。

### 对比现有方案

| | 现在 | 迁移后 |
|--|------|--------|
| Server→Node 认证 | server 持有客户端证书，node 验证 | Node 持有 CA 签发证书，server 验证 |
| Node 证书来源 | 自签，server 首次 TLS 握手时拉取 | Enrollment 时由 server CA 签发 |
| 证书分发 | 手动粘贴 server_client_cert.pem | 自动（enrollment 流程） |

---

## Proto 定义

```protobuf
syntax = "proto3";
package pulse.node.v1;

// Server 发起调用，Node 响应
service NodeAgent {
  // 核心双向流：server 发指令，node 回响应；node 也可主动推送
  rpc Session(stream NodeMessage) returns (stream ServerMessage);
}

message ServerMessage {
  string id     = 1;  // 请求 ID，用于关联响应
  string method = 2;  // 对应现有 Client 方法名
  bytes  body   = 3;  // JSON 编码的请求体（复用现有结构体）
  // 用于取消正在进行的流式调用（如 LogsStream）
  string cancel_id = 4;
}

message NodeMessage {
  string id    = 1;  // 对应请求 ID（主动推送时为空）
  bool   ok    = 2;
  bytes  body  = 3;  // JSON 编码的响应体
  string error = 4;
  // 主动推送（日志、traceroute 分片、定时 usage 上报）
  string event = 5;  // "log" | "traceroute_hop" | "usage_push" | "hello" | ""
  // node 主动推 usage 时携带，server ack 后 node 才能丢弃
  uint64 seq   = 6;
}
```

Body 复用现有 Go 结构体（`nodes.UsageStats`、`nodes.ConfigRequest` 等），JSON 序列化，protobuf 只做传输框架。后续有余力可全量迁移到 protobuf 类型，届时上面表格中"类型安全"的收益才能兑现。

---

## 通信模式细化

### SyncUsage：反转方向（node 主动推）

**不要把 pull 直接换成 gRPC pull**——那只是把 HTTP 替换成 gRPC，没发挥 push 模型优势。

| 现状 | 迁移后 |
|------|--------|
| Server 每分钟 `Call("SyncUsage")` | Node 每分钟主动推 `event=usage_push` 帧，带递增 `seq` |
| Server 拉失败 → 下一轮重试 | Server 收到后立即 ack（在 ServerMessage 里回 `id`），node 收到 ack 才丢弃本地缓冲；未 ack 的窗口在重连后重发 |
| Server 重启时正在进行的拉取丢失 | Node 持有未 ack 的 delta，server 重启后自动补齐 |

收益：
- Server 无状态化、重启零数据丢失
- Node 离线期间累积的流量在重连后自动追平（无需额外补偿逻辑）
- Server 不需要遍历节点发起拉取，定时器可移除

### 离线指令补偿

`ApplyNodeUsers` / `AddUser` / `RemoveUser` 在 node 离线时返回 `ErrNodeOffline`。重连后必须自动追上，否则管理员中途的变更会丢。

**机制**：node 重连后第一帧发送 `event=hello`，携带本地当前配置的 SHA256（对 node 上 xray 配置中的用户列表 + inbound 倍率等关键字段做规范化序列化后哈希）。Server 在内存中维护 DB 当前配置的 hash（写路径变更时失效重算），比对不一致则主动 `Call("ApplyNodeUsers")` 推完整配置。

这等价于 node 重连时的 self-sync，无需在 server 侧维护离线指令队列，也不动 schema。

### Usage push 的 ack-before-reset

现有 `Client.Usage(ctx, reset=true)` 让 node 上报后立即清零 xray 计数器。改为 push 模型后必须**先 ack 再 reset**，否则：

```
node push usage{seq=5, delta=100MB} → 网络丢包 → server 没收到
node 已 reset xray 计数器 → 100MB 永久丢失
```

正确流程：
1. Node push `usage_push{seq=N, body=delta}` 但**不 reset**
2. Server 写库成功后回 `ServerMessage{id=N, ok=true}` 作为 ack
3. Node 收到 ack 后才 reset xray 计数器并清掉本地缓冲
4. 未 ack 的 seq 在重连后从最小值开始重发；server 用 seq 去重

### SSE / 长流的取消

原 `LogsStream` / `TracerouteStream` 是 HTTP SSE，前端断开 server 自然感知。迁移后链路是 `前端 SSE ↔ server ↔ gRPC stream ↔ node`，前端断开后 server 必须主动告诉 node 停止。

机制：proto 中新增 `ServerMessage.cancel_id` 字段，server 检测到 SSE writer 断开后发送 `{cancel_id: <orig_req_id>}`，node 侧 dispatcher 终止对应 goroutine。否则 node 上长流会泄漏。

---

## 架构权衡（必须明确接受的代价）

### Stateful 化是单向门

现状 pull 模型下 server 完全无状态，可以随便水平扩展（多实例 + 共享 DB）。改成 push 后：

- Node 的 gRPC 长连接绑定到**具体的 server 进程**（不是 server 集群）
- 任何一条指令都必须路由到**持有该连接的那个进程**
- 如果将来要水平扩展，必须引入：
  - Redis pub/sub 或 NATS 做指令路由（按 nodeID 找到持有连接的实例）
  - 健康检查 + 故障转移（实例挂了，node 自动重连到另一个实例）
  - 跨实例的 SSE 转发链

**这是架构层面的单向门**——一旦切到 push，回不到无状态 server。

如果项目长期只有单 server 实例，这个代价可以接受；否则必须先想清楚多实例方案再动手。

### 改动面体量

"可删除"那段相当于**整个 `internal/nodeapi/` 包都要删**，加上 `nodes/client.go` 重写、`install.sh` 重写、面板"添加节点"流程重做。比"修改"列出的范围大，需要预留 sniproxy 路由同步路径的迁移工作量。

---

## 改动范围

### 新增

| 文件 | 内容 |
|------|------|
| `proto/node/v1/node.proto` | 上述 proto 定义 |
| `internal/nodehub/hub.go` | Server 侧连接注册表（nodeID → gRPC stream） |
| `internal/nodehub/call.go` | RPC 封装：发送指令并等待响应，带超时 |
| `internal/nodeagent/agent.go` | Node 侧 gRPC 客户端，含指数退避重连 |
| `internal/nodeagent/dispatch.go` | Node 侧消息分发（method → 现有 handler） |
| `internal/nodeagent/enroll.go` | Enrollment 流程（生成 CSR → 获取证书） |
| `internal/serverapi/enroll.go` | Enrollment 端点 `POST /v1/node-enroll` |
| `internal/cert/ca.go` | Node CA 生成与证书签发 |

### 修改

| 文件 | 改动 |
|------|------|
| `internal/nodes/client.go` | 所有方法改为通过 Hub 发 gRPC 消息；接口签名不变 |
| `internal/node/server.go` | 移除 `ListenAndServeTLS`；启动 `nodeagent` |
| `internal/server/server.go` | 启动 `nodehub` gRPC server（`:8082`）；注册 enrollment 端点 |
| `internal/config/config.go` | 新增 `NodeAgentToken`、`NodeGRPCServerURL`；移除 mTLS 文件路径配置 |
| `install.sh` | 生成 CSR、调 enrollment、保存证书；移除手动粘贴证书步骤 |
| `internal/serverapi/node_register.go` | enrollment 时自动注册 BaseURL（node 发起连接时即知道身份） |

### 可删除（阶段三）

- `internal/node/server.go` TLS 监听部分
- `internal/nodes/client.go` HTTP client 构建代码（`buildHTTPClient`、`fetchServerCertificatePEM`）
- `internal/cert/selfsigned.go` node 自签证书逻辑
- `PULSE_NODE_TLS_*` 环境变量
- `PULSE_SERVER_NODE_CLIENT_CERT_FILE` / `KEY_FILE` 环境变量

---

## 迁移阶段

### 阶段一：基础设施（不影响现有功能）

1. 实现 Node CA（`internal/cert/ca.go`）
2. 实现 enrollment 端点（`POST /v1/node-enroll`）
3. 定义 proto，生成 Go 代码
4. 实现 `nodehub`：gRPC server + 连接注册表
5. 实现 `nodeagent`：gRPC 客户端 + enrollment 流程
6. **Node 同时保留 HTTP 监听**（双模式），确保回滚路径

### 阶段二：替换核心通信

1. `nodes.Client` 所有方法改走 Hub（接口签名不变，调用方零改动）
2. `SyncUsage` 验证：流量数据正确性、reset 语义
3. `ApplyNodeUsers` / `AddUser` / `RemoveUser` 路径验证
4. SSE 流（LogsStream / Traceroute）改为 gRPC 推送帧 → 转发给 HTTP SSE 客户端
5. 集成测试适配（用 gRPC test server 替代 httptest）

### 阶段三：清理

1. 移除 Node 侧 HTTP 监听
2. 移除旧 mTLS 配置和自签证书逻辑
3. 更新安装脚本
4. 更新 CLAUDE.md 架构说明

---

## 关键实现细节

### 长连接 keepalive 与死连接清理

为防止 NAT/中间设备静默丢弃长流，并快速发现死连接，server / node 双端均开启 gRPC keepalive：

| 参数 | 默认值 | 含义 |
|------|--------|------|
| `ServerOptions.KeepaliveTime` | 30s | 服务端 idle 超过此时长后主动 ping client |
| `ServerOptions.KeepaliveTimeout` | 10s | ping 后等待 ack 的超时；超时后 server 主动断开 |
| `ServerOptions.MinClientPingInterval` | 25s | EnforcementPolicy.MinTime；client ping 比此更频会被 grpc 标记为 too_many_pings |
| `ServerOptions.PermitWithoutStream` | true | 允许 client 在没有活跃 stream 时仍发 ping（与 reaper 配合判活） |
| `Options.DeadConnectionTimeout` | 60s | reaper 判定死连接的最长无帧时间 |
| `Options.ReaperInterval` | 10s | reaper 扫描间隔（仅测试调） |
| `nodeagent.Config.KeepaliveTime` | 30s | client 主动 ping 间隔 |
| `nodeagent.Config.KeepaliveTimeout` | 10s | client ping ack 超时 |
| `PermitWithoutStream`（client）| true | 与 server EnforcementPolicy 对齐 |

Hub 在 `RunReaper(ctx)` 中每 `ReaperInterval` 扫描 `perNodeLastSeen`，超过 `DeadConnectionTimeout` 没收到任何帧的 conn 被强制 `close()`，Session goroutine 通过 `select c.closed` 退出，gRPC 自动断流。`Snapshot.ReapedTotal` 暴露被回收的连接数。

#### 压测

`internal/nodehub/loadtest`（build tag `loadtest`）跑 1000 个 nodeagent 同进程 bufconn 连 hub，hold 30s 期间持续以 100 RPS 调 `hub.Call`，并验证：

- 所有节点能在 60s 内全部上线；
- 关停后 `hub.OnlineNodes()` 归零；
- GC 后 `runtime.NumGoroutine()` ≤ 200（实测 ~4，基线 2）；
- 内存峰值上限设为 4GB（同进程 1000 grpc client 的固有开销，并非泄漏；真实部署中 1000 nodes 分布在不同机器，server 侧占用远小于此）。

运行：

```bash
make loadtest
# 等价：go test -tags loadtest -count=1 -timeout 5m ./internal/nodehub/loadtest/...
```

### Node 重连（指数退避）

```go
// nodeagent/agent.go
func Run(ctx context.Context, cfg Config) {
    bo := []time.Duration{2*time.Second, 5*time.Second, 15*time.Second, 60*time.Second}
    attempts := 0
    for {
        err := runSession(ctx, cfg)
        if ctx.Err() != nil { return }
        wait := bo[min(attempts, len(bo)-1)]
        log.Printf("node agent: reconnecting in %s: %v", wait, err)
        time.Sleep(wait)
        attempts++
    }
}
```

### Server 侧 RPC（请求-响应关联）

```go
// nodehub/call.go
func (h *Hub) Call(ctx context.Context, nodeID, method string, reqBody any) (json.RawMessage, error) {
    conn, ok := h.conns.Load(nodeID)
    if !ok {
        return nil, ErrNodeOffline
    }
    id := newID()
    ch := make(chan *NodeMessage, 1)
    h.pending.Store(id, ch)
    defer h.pending.Delete(id)

    conn.Send(&ServerMessage{Id: id, Method: method, Body: marshalJSON(reqBody)})
    select {
    case msg := <-ch:
        if !msg.Ok { return nil, errors.New(msg.Error) }
        return msg.Body, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

### SSE 流转发

原 `LogsStream` / `TracerouteStream` 是 node 返回的 HTTP SSE。迁移后：
- Node 收到指令后，通过 gRPC 流持续发送 `event="log"` 帧
- Server 侧收到推送帧，写入等待该请求的 HTTP SSE ResponseWriter

---

## 风险点

| 风险 | 缓解 |
|------|------|
| gRPC 引入新依赖 + 工具链 | `google.golang.org/grpc` 成熟稳定；`protoc` 加入 Makefile（与 sqlc 并列），`make generate` 一键生成 |
| **Stateful 化后多实例水平扩展受阻** | **架构层面单向门**，详见"架构权衡"。短期单实例可接受，长期需 Redis/NATS 路由层 |
| **CDN/L7 部署不兼容** | gRPC 必须直连，提供独立域名/端口；文档化部署要求 |
| Node 重连期间指令丢失 | hello 帧 + self-sync 补偿；`SyncUsage` 改 push 后无丢失 |
| 长流（log/traceroute）goroutine 泄漏 | proto 新增 `cancel_id`，server 检测 SSE 断开后下发取消 |
| 长耗时指令（测速）超时 | 按 method 配置独立 context timeout，与现有 `doLong` 语义对应 |
| Enrollment token 安全 | 一次性 + 1 小时过期 + 预绑 node_id；从 stdin/`--token-file` 读取，不进 shell history |
| Node CA 私钥泄漏 | 文件权限 0600；后续可接入 KMS/HSM；定期轮换 CA 流程需另行设计 |
| 大规模节点心跳压力 | 单 server 实例支持的活跃连接上限需压测；预留心跳间隔/keepalive 调参（见"长连接 keepalive 与死连接清理"，已通过 1000 节点 bufconn 压测） |
