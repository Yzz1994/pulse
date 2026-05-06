# 开发指南

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

## 发布新版本

交互式选择 patch / minor / major，先运行测试，通过后自动打 tag 并推送，触发 GitHub Actions 构建：

```bash
make release
```
