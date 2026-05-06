package ixdomain

import "errors"

var ErrIXDomainNotFound = errors.New("ix domain not found")

// IXDomain 代表一条国内中转域名配置。
type IXDomain struct {
	ID     string `json:"id"`
	Name   string `json:"name"`   // 显示名称，如 "华东 IX"
	Domain string `json:"domain"` // 绑定域名，如 relay.example.cn
	Remark string `json:"remark"`
}

// Store 定义 IX 域名的持久化接口。
type Store interface {
	UpsertIXDomain(d IXDomain) (IXDomain, error)
	GetIXDomain(id string) (IXDomain, error)
	ListIXDomains() ([]IXDomain, error)
	DeleteIXDomain(id string) error
}
