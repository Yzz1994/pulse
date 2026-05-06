package auditrules

import "time"

// RuleType 审计规则类型。
type RuleType string

const (
	RuleTypeDomainKeyword RuleType = "domain_keyword" // 目标域名包含关键词
	RuleTypePort          RuleType = "port"            // 目标端口命中
	RuleTypeIP            RuleType = "ip"              // 目标 IP 精确匹配
)

// Rule 一条审计规则。
type Rule struct {
	ID        string    `json:"id"`
	Type      RuleType  `json:"type"`
	Value     string    `json:"value"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	List() ([]Rule, error)
	Insert(r Rule) error
	Delete(id string) error
	SetEnabled(id string, enabled bool) error
}
