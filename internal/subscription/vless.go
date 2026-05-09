package subscription

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"pulse/internal/inbounds"
	"pulse/internal/users"
)

// rateAnnotationRe 匹配末尾的流量倍率标注，如 " [1.5x]" 或 "[2x]"。
var rateAnnotationRe = regexp.MustCompile(`\s*\[\d+\.?\d*x\]\s*$`)

// buildName 生成订阅链接中的节点显示名称。
// 优先级：Host.Remark（手动覆盖）> Country/Region/Network/Entry/Tags 组合 > Inbound.Tag > 连接地址
// num > 0 时在组合名末尾追加两位编号（同组多节点自动编号）。
// 流量倍率不为 1 时追加 [Nx]。
func buildName(ib inbounds.Inbound, host inbounds.Host, addr string, num int) string {
	// Remark 非空时直接用（手动覆盖），先剥掉已有的倍率标注再追加，避免重复
	if host.Remark != "" {
		name := strings.TrimSpace(rateAnnotationRe.ReplaceAllString(host.Remark, ""))
		if ib.TrafficRate != 0 && ib.TrafficRate != 1.0 {
			r := strconv.FormatFloat(ib.TrafficRate, 'f', -1, 32)
			name += " [" + r + "x]"
		}
		return name
	}

	var parts []string
	if host.Country != "" {
		parts = append(parts, host.Country)
	}
	if host.Region != "" {
		parts = append(parts, host.Region)
	}
	if host.Network != "" {
		parts = append(parts, host.Network)
	}
	if host.Entry != "" {
		parts = append(parts, host.Entry)
	}
	if host.Tags != "" {
		parts = append(parts, host.Tags)
	}
	if num > 0 {
		parts = append(parts, fmt.Sprintf("%02d", num))
	}

	var name string
	if len(parts) == 0 {
		// 无任何命名字段：回退旧逻辑
		name = ib.Tag
		if name == "" {
			name = addr
		}
	} else {
		name = strings.Join(parts, " ")
	}

	// 剥掉已有的倍率标注（Tags 等字段可能带有旧版写入的 [Nx]），再重新追加
	name = strings.TrimSpace(rateAnnotationRe.ReplaceAllString(name, ""))
	if ib.TrafficRate != 0 && ib.TrafficRate != 1.0 {
		r := strconv.FormatFloat(ib.TrafficRate, 'f', -1, 32)
		name += " [" + r + "x]"
	}
	return name
}

// nameKey 返回 host 的命名分组 key（用于自动编号）。
func nameKey(h inbounds.Host) string {
	if h.Country == "" && h.Region == "" && h.Network == "" && h.Entry == "" {
		return ""
	}
	return h.Country + "|" + h.Region + "|" + h.Network + "|" + h.Entry
}

type hostEntry struct {
	ib   inbounds.Inbound
	host inbounds.Host
	acc  users.UserInbound
}

// BuildLinks 为用户生成完整订阅链接列表，自动按命名分组编号。
// 同组（country+region+network+entry 相同）≥2 个 host 时追加 01/02… 编号。
// excluded 为用户已排除的 host ID 集合（nil 表示不过滤）。
func BuildLinks(accesses []users.UserInbound, ibStore inbounds.InboundStore, user users.User, excluded map[string]bool) []string {
	var entries []hostEntry
	seenInbound := make(map[string]bool)
	for _, acc := range accesses {
		if acc.InboundID == "" {
			nodeIbs, err := ibStore.ListInboundsByNode(acc.NodeID)
			if err != nil {
				continue
			}
			for _, ib := range nodeIbs {
				hosts, err := ibStore.ListHostsByInbound(ib.ID)
				if err != nil {
					continue
				}
				for _, h := range hosts {
					if excluded[h.ID] {
						continue
					}
					entries = append(entries, hostEntry{ib, h, acc})
				}
			}
			continue
		}
		// 同一个 inbound 可能同时有直接分配和组分配，只取第一条（凭据已复用，内容相同）
		if seenInbound[acc.InboundID] {
			continue
		}
		seenInbound[acc.InboundID] = true
		ib, err := ibStore.GetInbound(acc.InboundID)
		if err != nil {
			continue
		}
		hosts, err := ibStore.ListHostsByInbound(ib.ID)
		if err != nil {
			continue
		}
		for _, h := range hosts {
			if excluded[h.ID] {
				continue
			}
			entries = append(entries, hostEntry{ib, h, acc})
		}
	}

	// 按 country → region → network → entry 排序，同组内保持原顺序
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i].host, entries[j].host
		if a.Country != b.Country {
			return a.Country < b.Country
		}
		if a.Region != b.Region {
			return a.Region < b.Region
		}
		if a.Network != b.Network {
			return a.Network < b.Network
		}
		return a.Entry < b.Entry
	})

	// 统计每个命名 key 的出现次数（空 key 不参与编号）
	keyCounts := make(map[string]int)
	for _, e := range entries {
		if k := nameKey(e.host); k != "" {
			keyCounts[k]++
		}
	}

	// 生成链接，同组 ≥2 时附上递增编号
	keyIdx := make(map[string]int)
	var links []string
	for _, e := range entries {
		k := nameKey(e.host)
		num := 0
		if k != "" && keyCounts[k] > 1 {
			keyIdx[k]++
			num = keyIdx[k]
		}
		addr := e.host.Address
		port := e.host.Port
		name := buildName(e.ib, e.host, addr, num)
		acc := effectiveAccess(e.acc, user)
		var link string
		switch e.ib.Protocol {
		case "trojan":
			link = trojanLink(e.ib, e.host, acc, name, addr, port)
		case "shadowsocks":
			link = shadowsocksLink(e.ib, e.host, acc, name, addr, port)
		case "anytls":
			link = anytlsLink(e.ib, e.host, acc, name, addr, port)
		case "hy2":
			link = hy2Link(e.ib, e.host, acc, name, addr, port)
		default:
			link = vlessLink(e.ib, e.host, acc, name, addr, port)
		}
		if link != "" {
			links = append(links, link)
		}
	}
	return links
}

// Link 根据节点 inbound、客户端 host 模板、用户凭据和用户信息生成订阅链接。
// effectiveAccess 返回使用全局凭证覆盖后的 UserInbound，保持调用方无感知。
func effectiveAccess(acc users.UserInbound, user users.User) users.UserInbound {
	if user.UUID != "" {
		acc.UUID = user.UUID
	}
	if user.Secret != "" {
		acc.Secret = user.Secret
	}
	return acc
}

func Link(nodeInbound inbounds.Inbound, host inbounds.Host, access users.UserInbound, user users.User) string {
	access = effectiveAccess(access, user)
	// 连接地址和端口
	addr := host.Address
	port := host.Port
	name := buildName(nodeInbound, host, addr, 0)

	switch nodeInbound.Protocol {
	case "trojan":
		return trojanLink(nodeInbound, host, access, name, addr, port)
	case "shadowsocks":
		return shadowsocksLink(nodeInbound, host, access, name, addr, port)
	case "anytls":
		return anytlsLink(nodeInbound, host, access, name, addr, port)
	case "hy2":
		return hy2Link(nodeInbound, host, access, name, addr, port)
	default: // vless
		return vlessLink(nodeInbound, host, access, name, addr, port)
	}
}

func vlessLink(ib inbounds.Inbound, host inbounds.Host, acc users.UserInbound, username, addr string, port int) string {
	query := url.Values{}
	query.Set("type", "tcp")

	security := host.Security
	if security == "" || security == "__inherit__" {
		security = ib.Security
	}

	if security == "reality" {
		pubkey := host.RealityPublicKey
		if pubkey == "" {
			pubkey = ib.RealityPublicKey
		}
		shortID := host.RealityShortID
		if shortID == "" {
			shortID = ib.RealityShortID
		}
		spiderX := host.RealitySpiderX

		query.Set("security", "reality")
		query.Set("pbk", pubkey)
		query.Set("sid", shortID)
		if spiderX != "" {
			query.Set("spx", spiderX)
		}
		sni := ""
		if ib.RealityHandshakeAddr != "" {
			if h, _, err := net.SplitHostPort(ib.RealityHandshakeAddr); err == nil {
				sni = h
			} else {
				sni = ib.RealityHandshakeAddr
			}
		}
		if sni == "" {
			sni = host.SNI
		}
		if sni == "" {
			sni = addr
		}
		query.Set("sni", sni)
		fp := host.Fingerprint
		if fp == "" {
			fp = "chrome"
		}
		query.Set("fp", fp)
		query.Set("flow", "xtls-rprx-vision")
	} else {
		query.Set("security", "none")
	}

	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		acc.UUID, addr, port, query.Encode(), url.PathEscape(username),
	)
}

func trojanLink(ib inbounds.Inbound, host inbounds.Host, acc users.UserInbound, username, addr string, port int) string {
	query := url.Values{}
	query.Set("type", "ws")
	query.Set("path", "/ws")
	query.Set("security", "tls")
	query.Set("sni", addr)
	if host.Fingerprint != "" {
		query.Set("fp", host.Fingerprint)
	}

	return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		acc.Secret, addr, port, query.Encode(), url.PathEscape(username),
	)
}

// anytlsLink 生成 AnyTLS 订阅链接：anytls://<password>@<host>:<port>?sni=<sni>#<name>
func anytlsLink(ib inbounds.Inbound, host inbounds.Host, acc users.UserInbound, username, addr string, port int) string {
	query := url.Values{}
	sni := host.SNI
	if sni == "" {
		sni = addr
	}
	query.Set("sni", sni)
	if host.AllowInsecure {
		query.Set("insecure", "1")
	}
	return fmt.Sprintf("anytls://%s@%s:%d?%s#%s",
		url.PathEscape(acc.Secret), addr, port, query.Encode(), url.PathEscape(username),
	)
}

// hy2Link 生成 hysteria2 订阅链接：hysteria2://<password>@<host>:<port>?sni=...&insecure=...&obfs=...&obfs-password=...#<name>
// 客户端兼容：v2rayN / NekoBox / hiddify / Stash 均认 hysteria2:// scheme。
func hy2Link(ib inbounds.Inbound, host inbounds.Host, acc users.UserInbound, username, addr string, port int) string {
	query := url.Values{}
	sni := host.SNI
	if sni == "" {
		// hy2 SNI 优先用 Extra.sni（与服务端证书域名一致），其次回退到 host.Address
		if e := parseHy2ExtraForLink(ib.Extra); e.SNI != "" {
			sni = e.SNI
		} else {
			sni = addr
		}
	}
	query.Set("sni", sni)
	if host.AllowInsecure {
		query.Set("insecure", "1")
	} else {
		query.Set("insecure", "0")
	}
	// obfs：复用 ib.Method / ib.Password。Method == "salamander" 时启用混淆。
	if ib.Method == "salamander" && ib.Password != "" {
		query.Set("obfs", "salamander")
		query.Set("obfs-password", ib.Password)
	}
	if host.ALPN != "" {
		query.Set("alpn", host.ALPN)
	} else {
		query.Set("alpn", "h3")
	}
	return fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s",
		url.PathEscape(acc.Secret), addr, port, query.Encode(), url.PathEscape(username),
	)
}

// parseHy2ExtraForLink 与 proxycfg.parseHy2Extra 保持字段一致；
// 复制一份以避免 subscription 包反向依赖 proxycfg。
func parseHy2ExtraForLink(raw string) struct {
	SNI string `json:"sni"`
} {
	var e struct {
		SNI string `json:"sni"`
	}
	if raw == "" {
		return e
	}
	_ = json.Unmarshal([]byte(raw), &e)
	return e
}

func shadowsocksLink(ib inbounds.Inbound, host inbounds.Host, acc users.UserInbound, username, addr string, port int) string {
	method := ib.Method
	if method == "" {
		method = "aes-128-gcm"
	}
	userPSK := acc.Secret
	// SS 2022 user PSK 须与 xray 配置派生逻辑一致（HMAC-SHA256，指定字节长度）
	if strings.HasPrefix(method, "2022-") && userPSK != "" {
		keyLen := 16
		if strings.Contains(method, "256") || strings.Contains(method, "chacha20") {
			keyLen = 32
		}
		mac := hmac.New(sha256.New, []byte(userPSK))
		mac.Write([]byte(fmt.Sprintf("ss2022-%d", keyLen)))
		userPSK = base64.StdEncoding.EncodeToString(mac.Sum(nil)[:keyLen])
	}
	var credentials string
	if strings.HasPrefix(method, "2022-") {
		credentials = fmt.Sprintf("%s:%s:%s", method, ib.Password, userPSK)
	} else {
		credentials = fmt.Sprintf("%s:%s", method, userPSK)
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(credentials))
	return fmt.Sprintf("ss://%s@%s:%d#%s", encoded, addr, port, url.PathEscape(username))
}
