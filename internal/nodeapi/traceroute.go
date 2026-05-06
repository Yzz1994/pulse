package nodeapi

import (
	"context"
	"fmt"
	"net"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// TracerouteHop 单跳追踪结果。
type TracerouteHop struct {
	Hop     int       `json:"hop"`
	IP      string    `json:"ip,omitempty"`
	RttMs   []float64 `json:"rtt_ms,omitempty"`
	Timeout bool      `json:"timeout,omitempty"`
}

const (
	tracerouteMaxHops    = 30
	tracerouteProbes     = 3
	tracerouteTimeout    = 1 * time.Second // 每 probe 超时；超时跳总等待 = probes × timeout
	tracerouteTotalLimit = 120 * time.Second
)

// TracerouteRequest 是 traceroute 调用的输入参数。
type TracerouteRequest struct {
	Host   string `json:"host"`
	Method string `json:"method"` // "icmp" | "tcp"；其他值视为 "icmp"
	Port   int    `json:"port"`   // 仅 tcp 模式有效；<=0 时默认 80
}

// TracerouteEvent 是 TracerouteHops 通道中流出的一帧。
// Hop != nil 表示一跳；Err != "" 表示发生错误（之后通道随即关闭）。
type TracerouteEvent struct {
	Hop *TracerouteHop
	Err string
}

// TracerouteHops 启动一次 traceroute，将每跳与终态（错误/完成）通过 channel 投递。
// 通道在追踪结束（reach 或超时）或 ctx 取消后关闭。
func (a *API) TracerouteHops(ctx context.Context, req TracerouteRequest) <-chan TracerouteEvent {
	out := make(chan TracerouteEvent, tracerouteMaxHops+2)
	go func() {
		defer close(out)
		host := strings.TrimSpace(req.Host)
		if host == "" {
			out <- TracerouteEvent{Err: "host 参数不能为空"}
			return
		}
		if strings.ContainsAny(host, " ;|&`$(){}[]<>\\'\"") {
			out <- TracerouteEvent{Err: "host 包含非法字符"}
			return
		}
		method := req.Method
		if method != "tcp" {
			method = "icmp"
		}
		port := req.Port
		if port <= 0 || port > 65535 {
			port = 80
		}

		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil || len(addrs) == 0 {
			out <- TracerouteEvent{Err: fmt.Sprintf("DNS 解析失败: %v", err)}
			return
		}
		destIP := net.ParseIP(addrs[0])
		isIPv6 := destIP.To4() == nil

		runCtx, cancel := context.WithTimeout(ctx, tracerouteTotalLimit)
		defer cancel()

		hopCh := make(chan TracerouteHop, tracerouteMaxHops)
		doneCh := make(chan error, 1)
		go func() {
			var e error
			if method == "tcp" {
				if isIPv6 {
					e = traceTCPv6(runCtx, destIP, port, hopCh)
				} else {
					e = traceTCPv4(runCtx, destIP.To4(), port, hopCh)
				}
			} else {
				if isIPv6 {
					e = traceICMPv6(runCtx, destIP, hopCh)
				} else {
					e = traceICMPv4(runCtx, destIP.To4(), hopCh)
				}
			}
			doneCh <- e
		}()

		for {
			select {
			case hop, ok := <-hopCh:
				if !ok {
					hopCh = nil
					continue
				}
				h := hop
				select {
				case out <- TracerouteEvent{Hop: &h}:
				case <-ctx.Done():
					return
				}
			case e := <-doneCh:
				// 排空剩余 hop
				for len(hopCh) > 0 {
					hop := <-hopCh
					h := hop
					select {
					case out <- TracerouteEvent{Hop: &h}:
					case <-ctx.Done():
						return
					}
				}
				if e != nil {
					out <- TracerouteEvent{Err: e.Error()}
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// ── ICMP traceroute ──────────────────────────────────────────────

func traceICMPv4(ctx context.Context, dest net.IP, hopCh chan<- TracerouteHop) error {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("创建 ICMP socket 失败（需要 root 权限）: %v", err)
	}
	defer conn.Close()

	id := uint16(time.Now().UnixNano() & 0xffff)

	for ttl := 1; ttl <= tracerouteMaxHops; ttl++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		hop, reached := probeICMPv4(conn, dest, ttl, id)
		hopCh <- hop
		if reached {
			break
		}
	}
	return nil
}

func probeICMPv4(conn *icmp.PacketConn, dest net.IP, ttl int, id uint16) (TracerouteHop, bool) {
	hop := TracerouteHop{Hop: ttl}
	var rtts []float64
	var hopIP string
	reached := false

	for probe := 0; probe < tracerouteProbes; probe++ {
		seq := uint16(ttl*tracerouteProbes + probe)
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{ID: int(id), Seq: int(seq)},
		}
		wb, _ := msg.Marshal(nil)
		if err := conn.IPv4PacketConn().SetTTL(ttl); err != nil {
			continue
		}
		start := time.Now()
		if _, err := conn.WriteTo(wb, &net.IPAddr{IP: dest}); err != nil {
			continue
		}
		conn.SetReadDeadline(time.Now().Add(tracerouteTimeout))
		rb := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(rb)
		if err != nil {
			continue
		}
		rtt := float64(time.Since(start).Microseconds()) / 1000.0
		rm, err := icmp.ParseMessage(1, rb[:n])
		if err != nil {
			continue
		}
		peerIP := peer.(*net.IPAddr).IP.String()
		switch rm.Type {
		case ipv4.ICMPTypeTimeExceeded:
			hopIP = peerIP
			rtts = append(rtts, rtt)
		case ipv4.ICMPTypeEchoReply:
			hopIP = peerIP
			rtts = append(rtts, rtt)
			reached = true
		}
	}

	if hopIP == "" {
		hop.Timeout = true
	} else {
		hop.IP = hopIP
		hop.RttMs = rtts
	}
	return hop, reached
}

func traceICMPv6(ctx context.Context, dest net.IP, hopCh chan<- TracerouteHop) error {
	conn, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return fmt.Errorf("创建 ICMPv6 socket 失败（需要 root 权限）: %v", err)
	}
	defer conn.Close()

	id := uint16(time.Now().UnixNano() & 0xffff)

	for ttl := 1; ttl <= tracerouteMaxHops; ttl++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		hop := TracerouteHop{Hop: ttl}
		var rtts []float64
		var hopIP string
		reached := false

		for probe := 0; probe < tracerouteProbes; probe++ {
			seq := uint16(ttl*tracerouteProbes + probe)
			msg := icmp.Message{
				Type: ipv6.ICMPTypeEchoRequest,
				Code: 0,
				Body: &icmp.Echo{ID: int(id), Seq: int(seq)},
			}
			wb, _ := msg.Marshal(nil)
			if err := conn.IPv6PacketConn().SetHopLimit(ttl); err != nil {
				continue
			}
			start := time.Now()
			if _, err := conn.WriteTo(wb, &net.IPAddr{IP: dest}); err != nil {
				continue
			}
			conn.SetReadDeadline(time.Now().Add(tracerouteTimeout))
			rb := make([]byte, 1500)
			n, peer, err := conn.ReadFrom(rb)
			if err != nil {
				continue
			}
			rtt := float64(time.Since(start).Microseconds()) / 1000.0
			rm, err := icmp.ParseMessage(58, rb[:n])
			if err != nil {
				continue
			}
			peerIP := peer.(*net.IPAddr).IP.String()
			switch rm.Type {
			case ipv6.ICMPTypeTimeExceeded:
				hopIP = peerIP
				rtts = append(rtts, rtt)
			case ipv6.ICMPTypeEchoReply:
				hopIP = peerIP
				rtts = append(rtts, rtt)
				reached = true
			}
		}

		if hopIP == "" {
			hop.Timeout = true
		} else {
			hop.IP = hopIP
			hop.RttMs = rtts
		}
		hopCh <- hop
		if reached {
			break
		}
	}
	return nil
}

// ── TCP traceroute ───────────────────────────────────────────────

func tcpTTLDialer(ttl int, timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			var setErr error
			_ = c.Control(func(fd uintptr) {
				setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
			})
			return setErr
		},
	}
}

func traceTCPv4(ctx context.Context, dest net.IP, dstPort int, hopCh chan<- TracerouteHop) error {
	icmpConn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("创建 ICMP socket 失败（需要 root 权限）: %v", err)
	}
	defer icmpConn.Close()

	addr := fmt.Sprintf("%s:%d", dest, dstPort)

	for ttl := 1; ttl <= tracerouteMaxHops; ttl++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		hop := TracerouteHop{Hop: ttl}
		var rtts []float64
		var hopIP string
		reached := false

		for probe := 0; probe < tracerouteProbes; probe++ {
			start := time.Now()

			dialCh := make(chan error, 1)
			go func() {
				c, err := tcpTTLDialer(ttl, tracerouteTimeout).DialContext(ctx, "tcp4", addr)
				if c != nil {
					c.Close()
				}
				dialCh <- err
			}()

			icmpConn.SetReadDeadline(time.Now().Add(tracerouteTimeout))
			rb := make([]byte, 1500)
			n, peer, icmpErr := icmpConn.ReadFrom(rb)
			rtt := float64(time.Since(start).Microseconds()) / 1000.0

			dr := <-dialCh

			if icmpErr == nil {
				rm, err := icmp.ParseMessage(1, rb[:n])
				if err == nil {
					if rm.Type == ipv4.ICMPTypeTimeExceeded {
						hopIP = peer.(*net.IPAddr).IP.String()
						rtts = append(rtts, rtt)
					}
				}
			}
			if dr == nil || isTCPRefused(dr) {
				hopIP = dest.String()
				rtts = append(rtts, rtt)
				reached = true
			}
		}

		if hopIP == "" {
			hop.Timeout = true
		} else {
			hop.IP = hopIP
			hop.RttMs = rtts
		}
		hopCh <- hop
		if reached {
			break
		}
	}
	return nil
}

func traceTCPv6(ctx context.Context, dest net.IP, dstPort int, hopCh chan<- TracerouteHop) error {
	icmpConn, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return fmt.Errorf("创建 ICMPv6 socket 失败（需要 root 权限）: %v", err)
	}
	defer icmpConn.Close()

	addr := fmt.Sprintf("[%s]:%d", dest, dstPort)

	for ttl := 1; ttl <= tracerouteMaxHops; ttl++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		hop := TracerouteHop{Hop: ttl}
		var rtts []float64
		var hopIP string
		reached := false

		for probe := 0; probe < tracerouteProbes; probe++ {
			start := time.Now()

			dialCh := make(chan error, 1)
			go func() {
				d := &net.Dialer{
					Timeout: tracerouteTimeout,
					Control: func(network, address string, c syscall.RawConn) error {
						var setErr error
						_ = c.Control(func(fd uintptr) {
							setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_UNICAST_HOPS, ttl)
						})
						return setErr
					},
				}
				c, err := d.DialContext(ctx, "tcp6", addr)
				if c != nil {
					c.Close()
				}
				dialCh <- err
			}()

			icmpConn.SetReadDeadline(time.Now().Add(tracerouteTimeout))
			rb := make([]byte, 1500)
			n, peer, icmpErr := icmpConn.ReadFrom(rb)
			rtt := float64(time.Since(start).Microseconds()) / 1000.0

			dialErr := <-dialCh

			if icmpErr == nil {
				rm, err := icmp.ParseMessage(58, rb[:n])
				if err == nil && rm.Type == ipv6.ICMPTypeTimeExceeded {
					hopIP = peer.(*net.IPAddr).IP.String()
					rtts = append(rtts, rtt)
				}
			}
			if dialErr == nil || isTCPRefused(dialErr) {
				hopIP = dest.String()
				rtts = append(rtts, rtt)
				reached = true
			}
		}

		if hopIP == "" {
			hop.Timeout = true
		} else {
			hop.IP = hopIP
			hop.RttMs = rtts
		}
		hopCh <- hop
		if reached {
			break
		}
	}
	return nil
}

func isTCPRefused(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "reset by peer")
}
