package nodedomain

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("node domain not found")

// NodeDomain 代表节点上用到的一条域名记录。
type NodeDomain struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`      // 关联节点，为空表示未分配
	CFDomainID string    `json:"cf_domain_id"` // 关联 CF 域名配置
	FQDN       string    `json:"fqdn"`         // 完整域名，如 jp.example.com
	RecordType string    `json:"record_type"`  // A、AAAA、CNAME
	Content    string    `json:"content"`      // IP 或 CNAME 目标
	Proxied    bool      `json:"proxied"`      // CF 代理开关
	SyncedAt   time.Time `json:"synced_at"`
}

// Store 定义节点域名的持久化接口。
type Store interface {
	Upsert(nd NodeDomain) (NodeDomain, error)
	List() ([]NodeDomain, error)
	ListByCFDomain(cfDomainID string) ([]NodeDomain, error)
	UpdateNodeID(id, nodeID string) (NodeDomain, error)
	Delete(id string) error
}
