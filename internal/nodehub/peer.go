package nodehub

import (
	"context"
	"errors"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// mtlsPeerExtractor 是默认的 PeerExtractor，从 grpc/credentials.TLSInfo 中提取
// client 证书 CN 作为 nodeID。
func mtlsPeerExtractor(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", errors.New("nodehub: no peer in context")
	}
	if p.AuthInfo == nil {
		return "", errors.New("nodehub: peer has no auth info (mTLS required)")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", errors.New("nodehub: peer auth info is not TLS")
	}
	return extractCNFromState(tlsInfo.State)
}
