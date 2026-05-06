# Pulse

分布式边缘节点编排与管理平台。单仓双进制：`pulse-server`（控制面）和 `pulse-node`（节点面），通过 mTLS 加密通信。

## 功能特性

- **多节点编排** — 控制面统一纳管任意数量的边缘节点，支持 RPC 热推送配置变更
- **资源配额与访问控制** — 流量配额、有效期、多状态机（active / disabled / limited / expired / on_hold）、按入站维度配置流量倍率
- **客户端配置分发** — 自动生成标准化配置端点，兼容主流客户端
- **NodeGate（内置网关）** — 节点进程内置 TLS 终止、ACME 自动证书（支持 Cloudflare DNS-01）与 HTTP 反向代理，无需额外部署 Nginx/Caddy
- **出站路由策略** — 在面板配置出站节点，实现多级流量转发
- **套餐与商店** — 套餐管理 + Stripe 付款，用户自助购买服务
- **用户分组** — 将用户划分至不同分组，批量管控节点访问权限
- **IP Sentinel** — 基于 IP 的连接频率限制与访问控制
- **GeoIP 路由规则** — 内置 GeoIP 数据库，支持按地区路由分流
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
