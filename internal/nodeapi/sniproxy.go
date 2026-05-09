package nodeapi

import (
"errors"
"log"

"pulse/internal/sniproxy"
)

// SNIProxySyncResponse 是 SyncSNIProxy 的成功响应。
type SNIProxySyncResponse struct {
Listen  string `json:"listen"`
Routes  int    `json:"routes"`
Managed bool   `json:"managed"`
}

// ErrSNIProxyNotConfigured 表示节点未启用内置 SNI 代理。
var ErrSNIProxyNotConfigured = errors.New("sni proxy manager not configured on this node")

// DoSyncSNIProxy 把完整 SNI 代理配置推给 manager 热更新。
func (a *API) DoSyncSNIProxy(req sniproxy.ManagerConfig) (SNIProxySyncResponse, error) {
if a.sniManager == nil {
return SNIProxySyncResponse{}, ErrSNIProxyNotConfigured
}
log.Printf("sniproxy apply: listen=%q routes=%d cf_token_set=%v",
req.Listen, len(req.Routes), req.CloudflareToken != "")
if err := a.sniManager.Apply(req); err != nil {
return SNIProxySyncResponse{}, err
}
return SNIProxySyncResponse{
Listen:  req.Listen,
Routes:  len(req.Routes),
Managed: a.sniManager.Config().Listen != "",
}, nil
}

// DoSNIProxyStatus 返回当前 SNI 代理状态摘要。脱敏 cloudflare token。
func (a *API) DoSNIProxyStatus() map[string]any {
if a.sniManager == nil {
return map[string]any{"enabled": false}
}
status := a.sniManager.Status()
cfg := a.sniManager.Config()
cfg.CloudflareToken = ""
return map[string]any{
"enabled": status.Listen != "",
"status":  status,
"config":  cfg,
}
}

// DoSNIProxyCertReady 返回 certmgr 已经在磁盘上落地的域名列表。
// panel 在下发 hy2 inbound 前用它判断 xray 直接加载证书是否会失败。
func (a *API) DoSNIProxyCertReady() map[string]any {
if a.sniManager == nil {
return map[string]any{"domains": []string{}}
}
return map[string]any{"domains": a.sniManager.ReadyCertDomains()}
}
