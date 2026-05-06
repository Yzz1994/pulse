package enrolltokens

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound 表示未找到指定的 enroll token。
	ErrNotFound = errors.New("enroll token not found")
	// ErrAlreadyConsumed 表示 token 已被消费。
	ErrAlreadyConsumed = errors.New("enroll token already consumed")
	// ErrExpired 表示 token 已过期。
	ErrExpired = errors.New("enroll token expired")
)

// Token 表示一条 enrollment 令牌记录。
type Token struct {
	Token      string
	NodeID     string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// Store 管理 enrollment 一次性令牌的持久化与原子消费。
type Store interface {
	// Insert 写入一条新 token。
	Insert(ctx context.Context, t Token) error
	// Get 按 token 查询完整记录；不存在返回 ErrNotFound。
	Get(ctx context.Context, token string) (Token, error)
	// Consume 原子消费 token：成功返回更新后的记录；
	// token 不存在返回 ErrNotFound，已消费返回 ErrAlreadyConsumed，已过期返回 ErrExpired。
	Consume(ctx context.Context, token string) (Token, error)
	// CleanupExpired 删除 expires_at < cutoff 的所有记录，返回删除条数。
	CleanupExpired(ctx context.Context, cutoff time.Time) (int, error)
}
