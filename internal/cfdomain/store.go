package cfdomain

import "errors"

var ErrCFDomainNotFound = errors.New("cf domain not found")

// CFDomain 代表一个受管理的 Cloudflare 域名配置。
type CFDomain struct {
	ID       string `json:"id"`
	CFToken  string `json:"cf_token,omitempty"` // CF API Token（API 返回时应脱敏）
	ZoneID   string `json:"zone_id"`            // CF Zone ID
	ZoneName string `json:"zone_name"`          // 域名，如 example.com
	Remark   string `json:"remark"`             // 备注
}

// MaskToken 返回脱敏后的 token，仅保留前 4 位和后 4 位。
func (d CFDomain) MaskToken() string {
	if len(d.CFToken) <= 8 {
		return "****"
	}
	return d.CFToken[:4] + "****" + d.CFToken[len(d.CFToken)-4:]
}

// Store 定义 CF 域名的持久化接口。
type Store interface {
	UpsertCFDomain(domain CFDomain) (CFDomain, error)
	GetCFDomain(id string) (CFDomain, error)
	ListCFDomains() ([]CFDomain, error)
	DeleteCFDomain(id string) error
}
