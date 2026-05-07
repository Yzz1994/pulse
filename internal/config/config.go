package config

import (
"os"
)

type Config struct {
ServerAddr  string
DataDir     string // PULSE_DATA_DIR，工作目录（geoip、uploads 等存放位置）
DatabaseURL string // PULSE_DATABASE_URL，PostgreSQL 连接串
WebDir      string

// Node CA（控制面用，签发 node 客户端证书）
NodeCACertFile string // PULSE_NODE_CA_CERT_FILE
NodeCAKeyFile  string // PULSE_NODE_CA_KEY_FILE

// 节点 gRPC（控制面）
// gRPC 与面板共用同一 TLS 端口（单端口模式），节点通过 enrollment 响应得知连接地址。
NodeGRPCURL string // PULSE_NODE_GRPC_URL

// 节点侧 gRPC 客户端配置（节点进程使用，由 enroll 写入）
NodeID            string // PULSE_NODE_ID
NodeServerAddr    string // PULSE_NODE_SERVER_ADDR，gRPC server host:port，例如 controlplane.example.com:8082
NodeClientCert    string // PULSE_NODE_CLIENT_CERT_FILE，enroll 写入的 node_cert.pem
NodeClientKey     string // PULSE_NODE_CLIENT_KEY_FILE，enroll 写入的 node_key.pem
NodeServerCAFile  string // PULSE_NODE_SERVER_CA_FILE，enroll 写入的 node_ca.pem
NodeServerName    string // PULSE_NODE_SERVER_NAME，可选；留空时从 NodeServerAddr 解析

// TLS 证书管理（certmagic，用于 Trojan 直连模式）
CertDir   string // certmagic 证书存储目录
ACMEEmail string // ACME 账号邮箱（Let's Encrypt 要求）
// Discourse SSO（可选）
DiscourseURL        string // PULSE_DISCOURSE_URL
DiscourseSSOSecret  string // PULSE_DISCOURSE_SSO_SECRET
DiscourseAdminUsers string // PULSE_DISCOURSE_ADMIN_USERS，逗号分隔；空则信任所有 Discourse 用户
// Stripe 支付（可选）
StripeSecretKey     string // PULSE_STRIPE_SECRET_KEY
StripeWebhookSecret string // PULSE_STRIPE_WEBHOOK_SECRET
}

func Load() Config {
return Config{
ServerAddr:          envOrDefault("PULSE_SERVER_ADDR", ":8080"),
DataDir:             envOrDefault("PULSE_DATA_DIR", "./data"),
DatabaseURL:         envOrDefault("PULSE_DATABASE_URL", ""),
WebDir:              envOrDefault("PULSE_WEB_DIR", ""),
NodeCACertFile:      envOrDefault("PULSE_NODE_CA_CERT_FILE", "./node_ca_cert.pem"),
NodeCAKeyFile:       envOrDefault("PULSE_NODE_CA_KEY_FILE", "./node_ca_key.pem"),
NodeGRPCURL:         envOrDefault("PULSE_NODE_GRPC_URL", ""),
NodeID:              envOrDefault("PULSE_NODE_ID", ""),
NodeServerAddr:      envOrDefault("PULSE_NODE_SERVER_ADDR", ""),
NodeClientCert:      envOrDefault("PULSE_NODE_CLIENT_CERT_FILE", "./node_cert.pem"),
NodeClientKey:       envOrDefault("PULSE_NODE_CLIENT_KEY_FILE", "./node_key.pem"),
NodeServerCAFile:    envOrDefault("PULSE_NODE_SERVER_CA_FILE", "./node_ca.pem"),
NodeServerName:      envOrDefault("PULSE_NODE_SERVER_NAME", ""),
CertDir:             envOrDefault("PULSE_CERT_DIR", "./certs"),
ACMEEmail:           envOrDefault("PULSE_ACME_EMAIL", ""),
DiscourseURL:        envOrDefault("PULSE_DISCOURSE_URL", ""),
DiscourseSSOSecret:  envOrDefault("PULSE_DISCOURSE_SSO_SECRET", ""),
DiscourseAdminUsers: envOrDefault("PULSE_DISCOURSE_ADMIN_USERS", ""),
StripeSecretKey:     envOrDefault("PULSE_STRIPE_SECRET_KEY", ""),
StripeWebhookSecret: envOrDefault("PULSE_STRIPE_WEBHOOK_SECRET", ""),
}
}

func envOrDefault(key, fallback string) string {
if value := os.Getenv(key); value != "" {
return value
}
return fallback
}
