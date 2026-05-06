# 开发指南

**依赖**：Go 1.21+、Bun、PostgreSQL

```bash
# 启动 server(:8080 HTTP + :8082 gRPC node hub) + React SPA dev server(:3000)
# pulse-node 不会自动启动，需通过 enroll 流程手动注册（见下）
make dev

# 停止所有开发进程
make stop

# 运行测试
make test

# nodehub gRPC 并发压测（1000 节点 bufconn）
make loadtest

# 编译所有二进制（同时构建并嵌入前端）
make build
```

默认地址：`http://localhost:3000`（面板，Vite dev 代理 API 到 :8080），账号 `admin` / 密码 `admin123`。

> `make dev` 使用硬编码的开发凭据，不要用于生产环境。
> 控制面会自动在 `dev-data/server/` 下生成 NodeCA（`node_ca_cert.pem` / `node_ca_key.pem`）。

## 本地注册一个节点

`make dev` 不会自动起 `pulse-node`。需要本地节点时分两步：

```bash
# 1) 通过面板「节点 → 添加节点」生成 enroll token，或调接口拿到 <TOKEN> 与 <ID>

# 2) 在另一个终端执行 enroll，再启动 pulse-node
./dist/pulse-node enroll \
  --server=http://localhost:8080 --node-id=<ID> --token=<TOKEN> \
  --insecure --out=./dev-data/node

PULSE_NODE_ID=<ID> \
PULSE_NODE_GRPC_URL=https://localhost:8082 \
PULSE_NODE_SERVER_ADDR=localhost:8082 \
PULSE_NODE_CLIENT_CERT_FILE=./dev-data/node/node_cert.pem \
PULSE_NODE_CLIENT_KEY_FILE=./dev-data/node/node_key.pem \
PULSE_NODE_SERVER_CA_FILE=./dev-data/node/node_ca.pem \
  ./dist/pulse-node
```

节点启动后会主动连 `localhost:8082` 建立 gRPC 长连接；进程本身不监听任何端口。

## 发布新版本

交互式选择 patch / minor / major，先运行测试，通过后自动打 tag 并推送，触发 GitHub Actions 构建：

```bash
make release
```
