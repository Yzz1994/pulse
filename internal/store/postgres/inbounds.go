package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/inbounds"
	"pulse/internal/store/postgres/sqlcgen"
)

// InboundStore 实现 inbounds.InboundStore 接口。
type InboundStore struct {
	db *pgxpool.Pool
}

func (db *DB) InboundStore() *InboundStore {
	return &InboundStore{db: db.conn}
}

// ─── Inbound ──────────────────────────────────────────────────────────────────

func (s *InboundStore) UpsertInbound(inbound inbounds.Inbound) (inbounds.Inbound, error) {
	if inbound.TrafficRate <= 0 {
		inbound.TrafficRate = 1.0
	}
	err := sqlcgen.New(s.db).UpsertInbound(context.Background(), sqlcgen.UpsertInboundParams{
		ID:                   inbound.ID,
		NodeID:               inbound.NodeID,
		Protocol:             inbound.Protocol,
		Tag:                  inbound.Tag,
		Port:                 int32(inbound.Port),
		Method:               inbound.Method,
		Password:             inbound.Password,
		Security:             inbound.Security,
		RealityPrivateKey:    inbound.RealityPrivateKey,
		RealityPublicKey:     inbound.RealityPublicKey,
		RealityHandshakeAddr: inbound.RealityHandshakeAddr,
		RealityShortID:       inbound.RealityShortID,
		OutboundID:           inbound.OutboundID,
		TrafficRate:          inbound.TrafficRate,
		TargetHost:           inbound.TargetHost,
		TargetPort:           int32(inbound.TargetPort),
	})
	if err != nil {
		return inbounds.Inbound{}, err
	}
	return inbound, nil
}

func (s *InboundStore) GetInbound(id string) (inbounds.Inbound, error) {
	row, err := sqlcgen.New(s.db).GetInboundByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return inbounds.Inbound{}, inbounds.ErrInboundNotFound
	}
	if err != nil {
		return inbounds.Inbound{}, err
	}
	return toInbound(row), nil
}

func (s *InboundStore) ListInbounds() ([]inbounds.Inbound, error) {
	rows, err := sqlcgen.New(s.db).ListInbounds(context.Background())
	if err != nil {
		return nil, err
	}
	return toInbounds(rows), nil
}

func (s *InboundStore) ListInboundsByNode(nodeID string) ([]inbounds.Inbound, error) {
	rows, err := sqlcgen.New(s.db).ListInboundsByNode(context.Background(), nodeID)
	if err != nil {
		return nil, err
	}
	return toInbounds(rows), nil
}

func (s *InboundStore) DeleteInbound(id string) error {
	res, err := sqlcgen.New(s.db).DeleteInboundByID(context.Background(), id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return inbounds.ErrInboundNotFound
	}
	// 清理关联的 user_inbounds 孤立记录
	_ = sqlcgen.New(s.db).DeleteUserInboundsByInboundID(context.Background(), id)
	return nil
}

// ─── Host ─────────────────────────────────────────────────────────────────────

func (s *InboundStore) UpsertHost(host inbounds.Host) (inbounds.Host, error) {
	err := sqlcgen.New(s.db).UpsertHost(context.Background(), sqlcgen.UpsertHostParams{
		ID:               host.ID,
		InboundID:        host.InboundID,
		Remark:           host.Remark,
		Address:          host.Address,
		Port:             int32(host.Port),
		Sni:              host.SNI,
		Host:             host.Host,
		Path:             host.Path,
		Security:         host.Security,
		Alpn:             host.ALPN,
		Fingerprint:      host.Fingerprint,
		AllowInsecure:    int32(boolToInt(host.AllowInsecure)),
		MuxEnable:        int32(boolToInt(host.MuxEnable)),
		RealityPublicKey: host.RealityPublicKey,
		RealityShortID:   host.RealityShortID,
		RealitySpiderX:   host.RealitySpiderX,
		Country:          host.Country,
		Region:           host.Region,
		Network:          host.Network,
		Entry:            host.Entry,
		Tags:             host.Tags,
		RelayNodeID:    host.RelayNodeID,
		HttpsPort: int32(host.HTTPSPort),
	})
	if err != nil {
		return inbounds.Host{}, err
	}
	return host, nil
}

func (s *InboundStore) GetHost(id string) (inbounds.Host, error) {
	row, err := sqlcgen.New(s.db).GetHostByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return inbounds.Host{}, inbounds.ErrHostNotFound
	}
	if err != nil {
		return inbounds.Host{}, err
	}
	return toHost(row), nil
}

func (s *InboundStore) ListHosts() ([]inbounds.Host, error) {
	rows, err := sqlcgen.New(s.db).ListHosts(context.Background())
	if err != nil {
		return nil, err
	}
	return toHosts(rows), nil
}

func (s *InboundStore) ListHostsByInbound(inboundID string) ([]inbounds.Host, error) {
	rows, err := sqlcgen.New(s.db).ListHostsByInbound(context.Background(), inboundID)
	if err != nil {
		return nil, err
	}
	return toHosts(rows), nil
}

func (s *InboundStore) DeleteHost(id string) error {
	res, err := sqlcgen.New(s.db).DeleteHostByID(context.Background(), id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return inbounds.ErrHostNotFound
	}
	return nil
}

// ─── 类型转换辅助 ─────────────────────────────────────────────────────────────

func toInbound(r sqlcgen.Inbound) inbounds.Inbound {
	in := inbounds.Inbound{
		ID:                   r.ID,
		NodeID:               r.NodeID,
		Protocol:             r.Protocol,
		Tag:                  r.Tag,
		Port:                 int(r.Port),
		Method:               r.Method,
		Password:             r.Password,
		Security:             r.Security,
		RealityPrivateKey:    r.RealityPrivateKey,
		RealityPublicKey:     r.RealityPublicKey,
		RealityHandshakeAddr: r.RealityHandshakeAddr,
		RealityShortID:       r.RealityShortID,
		OutboundID:           r.OutboundID,
		TrafficRate:          r.TrafficRate,
		TargetHost:           r.TargetHost,
		TargetPort:           int(r.TargetPort),
	}
	if in.TrafficRate <= 0 {
		in.TrafficRate = 1.0
	}
	return in
}

func toInbounds(rows []sqlcgen.Inbound) []inbounds.Inbound {
	out := make([]inbounds.Inbound, len(rows))
	for i, r := range rows {
		out[i] = toInbound(r)
	}
	return out
}

func toHost(r sqlcgen.Host) inbounds.Host {
	return inbounds.Host{
		ID:               r.ID,
		InboundID:        r.InboundID,
		Remark:           r.Remark,
		Address:          r.Address,
		Port:             int(r.Port),
		SNI:              r.Sni,
		Host:             r.Host,
		Path:             r.Path,
		Security:         r.Security,
		ALPN:             r.Alpn,
		Fingerprint:      r.Fingerprint,
		AllowInsecure:    r.AllowInsecure != 0,
		MuxEnable:        r.MuxEnable != 0,
		RealityPublicKey: r.RealityPublicKey,
		RealityShortID:   r.RealityShortID,
		RealitySpiderX:   r.RealitySpiderX,
		Country:          r.Country,
		Region:           r.Region,
		Network:          r.Network,
		Entry:            r.Entry,
		Tags:             r.Tags,
		RelayNodeID:    r.RelayNodeID,
		HTTPSPort: int(r.HttpsPort),
	}
}

func toHosts(rows []sqlcgen.Host) []inbounds.Host {
	out := make([]inbounds.Host, len(rows))
	for i, r := range rows {
		out[i] = toHost(r)
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
