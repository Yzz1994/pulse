package nodeapi

import (
"context"
"net"
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

// DoProbeLatency 并发 TCP connect 上海三网测延迟。nil 表示超时/失败。
func (a *API) DoProbeLatency(ctx context.Context) latencyProbeResult {
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
return result
}
