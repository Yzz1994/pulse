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

### 协议：gRPC（优于 WebSocket）

| 维度 | gRPC | WebSocket + JSON |
|------|------|-----------------|
| 双向流 | 原生支持 | 需手写请求-响应关联（id 对账） |
| 类型安全 | protobuf 编译期检查 | 无，运行时才发现 |
| 流控/背压 | HTTP/2 内置 | 需自实现 |
| 多路复用 | HTTP/2 内置 | 需自实现 |
| 超时传播 | context deadline 自动传递 | 需手动处理 |
| 生态 | `google.golang.org/grpc` 成熟稳定 | 无标准库，需第三方 |

relay 在完全相同的场景（master 控制 node）已验证 gRPC 可行，直接参考。

### 认证：mTLS（优于 Bearer Token）

| 维度 | mTLS | Bearer Token |
|------|------|-------------|
| 强度 | 双端密码学身份验证 | 单向，token 泄漏即失陷 |
| 轮换 | 证书有效期自动管理 | 需实现 token 轮换机制 |
| 基础设施 | pulse 已有自签证书逻辑 | 需新增 token 存储和验证 |
| 行业实践 | 服务间通信标准方案 | 适合用户侧 API |

---

## mTLS 证书方案

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

安装时执行一次性 enrollment：

```
1. Node 本地生成 RSA 私钥 + CSR
2. POST /v1/node-enroll  { node_id, csr_pem, enroll_token }
3. Server 验证 enroll_token，用 Node CA 签发证书，返回 cert_pem
4. Node 保存 cert_pem + key，后续 gRPC 连接使用此证书
```

`enroll_token` 是控制面生成安装命令时一并生成的一次性 token（存 DB，使用后失效），不是长期凭据。

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
  // 核心双向流：server 发指令，node 回响应
  rpc Session(stream ServerMessage) returns (stream NodeMessage);
}

message ServerMessage {
  string id     = 1;  // 请求 ID，用于关联响应
  string method = 2;  // 对应现有 Client 方法名
  bytes  body   = 3;  // JSON 编码的请求体（复用现有结构体）
}

message NodeMessage {
  string id    = 1;  // 对应请求 ID（主动推送时为空）
  bool   ok    = 2;
  bytes  body  = 3;  // JSON 编码的响应体
  string error = 4;
  // 主动推送（日志、traceroute 分片等）
  string event = 5;  // "log" | "traceroute_hop" | ""
}
```

Body 复用现有 Go 结构体（`nodes.UsageStats`、`nodes.ConfigRequest` 等），JSON 序列化，protobuf 只做传输框架。后续有余力可全量迁移到 protobuf 类型。

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
| gRPC 引入新依赖 | `google.golang.org/grpc` 是 Go 生态最成熟的包之一，无维护风险 |
| Node 重连期间指令丢失 | Hub 返回 `ErrNodeOffline`，`SyncUsage` 记录 dialErr，行为与现在一致 |
| 长耗时指令（测速）超时 | 按 method 配置独立 context timeout，与现有 `doLong` 语义对应 |
| Enrollment token 安全 | 一次性使用，使用后立即失效，有效期 1 小时 |
| 多 server 实例部署 | 阶段一不处理；后续可用 Redis pub/sub 将指令路由到持有连接的实例 |
