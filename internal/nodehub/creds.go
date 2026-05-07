package nodehub

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/soheilhy/cmux"
	"google.golang.org/grpc/credentials"
)

// passthroughTLSCreds implements credentials.TransportCredentials for
// connections already TLS-terminated by an outer tls.Listener.
//
// 单端口模式下 cmux 在 tls.Listener 之上按 content-type 路由：
// 每条连接到达 gRPC 子 listener 时是已完成握手的 *tls.Conn（被 cmux.MuxConn 包裹）。
// 此 credential 跳过重复的 TLS 握手，直接提取已有状态注入 peer.AuthInfo，
// 使 mtlsPeerExtractor 可正常读取客户端证书 CN。
type passthroughTLSCreds struct{}

var _ credentials.TransportCredentials = passthroughTLSCreds{}

func (passthroughTLSCreds) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, credentials.TLSInfo{State: extractTLSState(conn)}, nil
}

func (passthroughTLSCreds) ClientHandshake(_ context.Context, _ string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, nil, nil
}

func (passthroughTLSCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "tls"}
}

func (c passthroughTLSCreds) Clone() credentials.TransportCredentials { return c }
func (passthroughTLSCreds) OverrideServerName(string) error            { return nil }

// extractTLSState 从连接链中提取 TLS 状态。
// cmux.MuxConn 将原始 *tls.Conn 存储在导出字段 Conn 中。
func extractTLSState(conn net.Conn) tls.ConnectionState {
	if tc, ok := conn.(*tls.Conn); ok {
		return tc.ConnectionState()
	}
	if mc, ok := conn.(*cmux.MuxConn); ok {
		if tc, ok := mc.Conn.(*tls.Conn); ok {
			return tc.ConnectionState()
		}
	}
	return tls.ConnectionState{}
}
