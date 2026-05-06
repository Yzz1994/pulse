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

## 快速开始

**安装 server：**

```bash
curl -fsSL https://raw.githubusercontent.com/0xUnixIO/pulse/main/scripts/install.sh | sh -s -- server
```

**安装 node：**

在控制面板「节点」页面点击「添加节点」，填写节点名称后会自动生成安装命令，复制到节点机器上运行即可，无需手动配置证书。

完整安装步骤、环境变量配置、NodeGate 设置详见 [INSTALL.md](INSTALL.md)。

## 开发

详见 [DEVELOPMENT.md](DEVELOPMENT.md)。

## 许可证

[GNU Affero General Public License v3.0](LICENSE)
