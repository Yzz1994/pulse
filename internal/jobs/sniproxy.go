package jobs

import (
	"fmt"
	"strings"

	"pulse/internal/inbounds"
	"pulse/internal/nodes"
)

// BuildSNIProxySyncReq 从节点当前生效的 inbound/host 配置生成 NodeGate (sniproxy) 配置。
//
// 覆盖三种路由：
//   - Terminating：本节点 AnyTLS/Trojan inbound 的 SNI → 127.0.0.1:<xray_port>
//   - Transparent：其他节点以本节点为 Relay 的 Host SNI → <landing_ip>:<landing_port>
//   - HTTPReverse：node.PanelDomain（反代到 pulse-server panelPort）
//                  以及 node.ExtraProxies（每行 "domain:port" 反代到 127.0.0.1:port）
//
// 监听端口优先级：node.HTTPSPort > Host.HTTPSPort > 透明路由 ListenPort > 443。
func BuildSNIProxySyncReq(
	node nodes.Node,
	nodeInbounds []inbounds.Inbound,
	ibStore inbounds.InboundStore,
	allNodeMap map[string]nodes.Node,
	cfToken string,
	panelPort int,
) nodes.SNIProxySyncRequest {
	var routes []nodes.SNIProxyRoute
	hostHTTPSPort := 0

	// 1. 本节点 AnyTLS/Trojan inbound 路由：
	//    - NodeGate 模式（默认）：terminating，NodeGate 终止 TLS 转发明文给 Xray
	//    - direct 模式：transparent，NodeGate TCP passthrough，Xray 自持证书
	seenLocal := make(map[string]struct{})
	directMode := node.TLSMode == "direct"
	routeMode := "terminating"
	if directMode {
		routeMode = "transparent"
	}
	var relayCertDomains []string // direct 模式下 relay host 的 SNI：无路由但仍需证书
	for _, ib := range nodeInbounds {
		if ib.Protocol != "anytls" && ib.Protocol != "trojan" {
			continue
		}
		hosts, hErr := ibStore.ListHostsByInbound(ib.ID)
		if hErr != nil {
			continue
		}
		for _, h := range hosts {
			// Relay 的 Host：路由由前置节点负责，但 direct 模式下落地节点仍需证书
			if h.RelayNodeID != "" {
				if h.HTTPSPort > 0 && hostHTTPSPort == 0 {
					hostHTTPSPort = h.HTTPSPort
				}
				if directMode {
					sni := h.SNI
					if sni == "" {
						sni = h.Address
					}
					if sni != "" {
						if _, dup := seenLocal[sni]; !dup {
							seenLocal[sni] = struct{}{}
							relayCertDomains = append(relayCertDomains, sni)
						}
					}
				}
				continue
			}
			sni := h.SNI
			if sni == "" {
				sni = h.Address
			}
			if sni == "" {
				continue
			}
			if _, dup := seenLocal[sni]; dup {
				continue
			}
			seenLocal[sni] = struct{}{}
			routes = append(routes, nodes.SNIProxyRoute{
				SNI:     sni,
				Backend: fmt.Sprintf("127.0.0.1:%d", ib.Port),
				Mode:    routeMode,
			})
		}
	}

	// 2. Transparent 路由：其他节点的 Host 把本节点作为 RelayNodeID 时，
	//    在本节点 SNI 透传到落地节点 IP:port。
	if allHosts, err := ibStore.ListHosts(); err == nil {
		allIbs, _ := ibStore.ListInbounds()
		ibMap := make(map[string]inbounds.Inbound, len(allIbs))
		for _, ib := range allIbs {
			ibMap[ib.ID] = ib
		}
		nodeIPMap := BuildNodeIPMap(allNodeMap)
		seenTransparent := make(map[string]struct{})
		var transparentListenPort int
		for _, h := range allHosts {
			if h.RelayNodeID != node.ID || h.Port <= 0 {
				continue
			}
			ib, ok := ibMap[h.InboundID]
			if !ok {
				continue
			}
			targetIP := nodeIPMap[ib.NodeID]
			if targetIP == "" {
				continue
			}
			sni := h.SNI
			if sni == "" {
				sni = h.Address
			}
			if sni == "" {
				continue
			}
			if _, dup := seenTransparent[sni]; dup {
				continue
			}
			// 只接受与第一条透传路由 ListenPort 一致的 Host，其余跳过
			// （UnifiedProxy 目前单端口，不同监听端口的路由走不到这条通道）
			if transparentListenPort == 0 {
				transparentListenPort = h.Port
			}
			if h.Port != transparentListenPort {
				continue
			}
			seenTransparent[sni] = struct{}{}

			backendNode := allNodeMap[ib.NodeID]
			httpsPort := h.HTTPSPort
			if httpsPort == 0 {
				httpsPort = backendNode.HTTPSPort
			}
			// direct 模式下 NodeGate 透传，fallback 用 xray inbound 实际端口
			if httpsPort == 0 && backendNode.TLSMode == "direct" {
				httpsPort = ib.Port
			}
			targetPort := PortforwardTargetPort(ib, httpsPort)
			routes = append(routes, nodes.SNIProxyRoute{
				SNI:     sni,
				Backend: fmt.Sprintf("%s:%d", targetIP, targetPort),
				Mode:    "transparent",
			})
		}
	}

	// 3. HTTPReverse 路由：面板多域名（逗号/换行分隔）统一反代到 panelPort
	if node.PanelDomain != "" && panelPort > 0 {
		for _, d := range splitDomains(node.PanelDomain) {
			routes = append(routes, nodes.SNIProxyRoute{
				SNI:     d,
				Backend: fmt.Sprintf("127.0.0.1:%d", panelPort),
				Mode:    "http-reverse",
			})
		}
	}

	// 4. HTTPReverse 路由：用户自定义额外反代，每行一条
	// 新格式："domain host:port"（空格分隔，支持任意后端地址）
	// 旧格式（兼容）："domain:port"（仅端口，后端固定为 127.0.0.1）
	for _, line := range strings.Split(node.ExtraProxies, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var domain, backend string
		if spaceIdx := strings.Index(line, " "); spaceIdx > 0 {
			domain = strings.TrimSpace(line[:spaceIdx])
			backend = strings.TrimSpace(line[spaceIdx+1:])
		} else {
			colonIdx := strings.LastIndex(line, ":")
			if colonIdx <= 0 {
				continue
			}
			domain = strings.TrimSpace(line[:colonIdx])
			backend = "127.0.0.1:" + strings.TrimSpace(line[colonIdx+1:])
		}
		if domain == "" || backend == "" {
			continue
		}
		routes = append(routes, nodes.SNIProxyRoute{
			SNI:     domain,
			Backend: backend,
			Mode:    "http-reverse",
		})
	}

	// 监听端口决策
	listenPort := 0
	if node.HTTPSPort > 0 {
		listenPort = node.HTTPSPort
	} else if hostHTTPSPort > 0 {
		listenPort = hostHTTPSPort
	} else if len(routes) > 0 {
		listenPort = 443
	}

	// direct 模式下 Xray 自持证书，certmgr 需要管理：
	// 1. 所有透明路由 SNI（非 relay host）
	// 2. relay host 的 SNI（无路由但落地节点仍需证书）
	var certDomains []string
	if directMode {
		seen := make(map[string]struct{})
		for _, r := range routes {
			if r.Mode == "transparent" && r.SNI != "" {
				if _, dup := seen[r.SNI]; !dup {
					seen[r.SNI] = struct{}{}
					certDomains = append(certDomains, r.SNI)
				}
			}
		}
		for _, d := range relayCertDomains {
			if _, dup := seen[d]; !dup {
				seen[d] = struct{}{}
				certDomains = append(certDomains, d)
			}
		}
	}

	req := nodes.SNIProxySyncRequest{
		ACMEEmail:       node.ACMEEmail,
		CloudflareToken: cfToken,
		CertStoragePath: "/var/lib/pulse-node/certs",
		Routes:          routes,
		CertDomains:     certDomains,
	}
	if listenPort > 0 && len(routes) > 0 {
		req.Listen = fmt.Sprintf(":%d", listenPort)
	}
	return req
}

// splitDomains 把 PanelDomain 字符串按逗号/换行分割成干净的域名列表。
func splitDomains(s string) []string {
	var out []string
	for _, d := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}
