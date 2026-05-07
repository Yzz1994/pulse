# Pulse

分布式边缘节点编排与管理平台。`pulse-server` + `pulse-node` 双进程，节点通过 mTLS gRPC 长连接被控制面纳管，配置 push 秒级生效。

## 功能

- 多节点 mTLS 编排，配置 push 下发
- 用户额度 / 有效期 / 状态机 / 流量倍率
- 标准订阅链接生成
- 节点内置 TLS 网关（NodeGate）+ ACME（Cloudflare DNS-01）
- 多级出站路由 / GeoIP 分流
- 套餐 + Stripe 付款 / 用户分组 / 工单 / 审计日志 / IP 限频

## 安装

**Server**

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server
```

**Node**

面板「节点」→「添加节点」复制安装命令到节点机器运行。节点主动外连，NAT 机器可用。

详见 [INSTALL.md](INSTALL.md) · 开发参见 [DEVELOPMENT.md](DEVELOPMENT.md)。

## License

[AGPL-3.0](LICENSE)
