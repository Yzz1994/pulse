package proxycfg

import (
	"pulse/internal/inbounds"
	"pulse/internal/users"
)

// Build 生成 xray 配置 JSON。
func Build(nodeInbounds []inbounds.Inbound, userAccesses []users.UserInbound, userMap map[string]users.User, opts BuildOptions) (string, error) {
	return BuildXrayConfig(nodeInbounds, userAccesses, userMap, opts)
}

// BuildIdleFor 返回 xray 空闲配置字符串。
func BuildIdleFor() string {
	return xrayIdleConfig
}
