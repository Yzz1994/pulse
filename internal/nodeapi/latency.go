package nodeapi

import (
	"context"
	"net"
	"net/http"
	"time"
)

// shanghaiISPTargets 上海三网延迟测试目标（TCP connect）。
var shanghaiISPTargets = map[string]string{
	"ct": "sh-ct-v4.ip.zstaticcdn.com:80",
	"cu": "sh-cu-v4.ip.zstaticcdn.com:80",
	"cm": "sh-cm-v4.ip.zstaticcdn.com:80",
}

type latencyProbeResult struct {
	CT *int `json:"ct"`
	CU *int `json:"cu"`
	CM *int `json:"cm"`
}

// handleLatencyProbe 并发 TCP connect 上海三网，返回各 ISP 延迟（ms）。
// nil 表示连接超时或失败。
func (a *API) handleLatencyProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type probeResult struct {
		isp string
		rtt *int
	}

	ch := make(chan probeResult, len(shanghaiISPTargets))
	for isp, addr := range shanghaiISPTargets {
		go func(isp, addr string) {
			start := time.Now()
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
			if err != nil {
				ch <- probeResult{isp: isp, rtt: nil}
				return
			}
			conn.Close()
			ms := int(time.Since(start).Milliseconds())
			ch <- probeResult{isp: isp, rtt: &ms}
		}(isp, addr)
	}

	result := latencyProbeResult{}
	for range shanghaiISPTargets {
		r := <-ch
		switch r.isp {
		case "ct":
			result.CT = r.rtt
		case "cu":
			result.CU = r.rtt
		case "cm":
			result.CM = r.rtt
		}
	}

	writeJSON(w, http.StatusOK, result)
}
