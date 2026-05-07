import { useEffect, useMemo, useState, useCallback } from "react";
import { Link, useParams, useNavigate } from "@tanstack/react-router";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
  Badge,
  Button,
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
  toast,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";
import { formatBytes } from "@/lib/format";
import type { Node, Inbound, Host } from "@/lib/types";

interface TrafficPoint {
  date: string;
  upload_bytes: number;
  download_bytes: number;
}

interface TrafficResponse {
  days: number;
  points: TrafficPoint[];
}

interface SNIRoute {
  sni: string;
  backend: string;
  mode: string;
}

interface SNICertInfo {
  domain: string;
  not_before?: string;
  not_after?: string;
  issuer?: string;
  ready: boolean;
}

interface SNIStatus {
  listen: string;
  route_count: number;
  cert_domains: number;
  last_error?: string;
  routes?: SNIRoute[];
  certs?: SNICertInfo[];
}

interface SNIStatusResponse {
  enabled: boolean;
  status: SNIStatus;
  config?: { listen: string; acme_email?: string };
}

interface CheckResultItem {
  service: string;
  check_type: string;
  unlocked: boolean;
  region?: string;
  note?: string;
  checked_at: string;
}

interface SpeedTestResultItem {
  down_bps?: number;
  up_bps?: number;
  tested_at?: string;
}

interface TracerouteSnapshotItem {
  id: string;
  node_id: string;
  direction: string;
  target: string;
  hops: string;
  quality: string;
  created_at: string;
}

function fmtDate(s?: string | null): string {
  if (!s) return "—";
  const d = new Date(s);
  if (isNaN(d.getTime())) return s;
  return d.toISOString().slice(0, 10);
}

function buildDateAxis(days: number): string[] {
  const out: string[] = [];
  const today = new Date();
  today.setUTCHours(0, 0, 0, 0);
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setUTCDate(today.getUTCDate() - i);
    out.push(d.toISOString().slice(0, 10));
  }
  return out;
}

function TrafficChart({ points, days }: { points: TrafficPoint[]; days: number }) {
  const axis = useMemo(() => buildDateAxis(days), [days]);
  const map = useMemo(() => {
    const m = new Map<string, TrafficPoint>();
    points.forEach((p) => m.set(p.date, p));
    return m;
  }, [points]);
  const series = useMemo(() => {
    return axis.map((d) => {
      const p = map.get(d);
      return {
        date: d,
        up: p?.upload_bytes ?? 0,
        down: p?.download_bytes ?? 0,
        total: (p?.upload_bytes ?? 0) + (p?.download_bytes ?? 0),
      };
    });
  }, [axis, map]);
  const max = useMemo(() => Math.max(1, ...series.map((s) => s.total)), [series]);

  const grandTotal = series.reduce((acc, s) => acc + s.total, 0);
  const grandUp = series.reduce((acc, s) => acc + s.up, 0);
  const grandDown = series.reduce((acc, s) => acc + s.down, 0);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-[hsl(var(--muted-foreground))]">
        <span>合计 <span className="font-semibold text-foreground">{formatBytes(grandTotal)}</span></span>
        <span>上传 <span className="font-semibold text-foreground">{formatBytes(grandUp)}</span></span>
        <span>下载 <span className="font-semibold text-foreground">{formatBytes(grandDown)}</span></span>
        <span className="ml-auto">共 {days} 天</span>
      </div>
      <div className="flex h-40 items-end gap-px overflow-hidden rounded border bg-[hsl(var(--muted))/0.3] p-2">
        {series.map((s) => {
          const upPct = (s.up / max) * 100;
          const downPct = (s.down / max) * 100;
          return (
            <div
              key={s.date}
              className="group relative flex h-full flex-1 flex-col justify-end"
              title={`${s.date}\n上传: ${formatBytes(s.up)}\n下载: ${formatBytes(s.down)}\n合计: ${formatBytes(s.total)}`}
            >
              <div
                className="w-full bg-blue-500/70"
                style={{ height: `${downPct}%` }}
              />
              <div
                className="w-full bg-emerald-500/70"
                style={{ height: `${upPct}%` }}
              />
            </div>
          );
        })}
      </div>
      <div className="flex items-center justify-between text-[10px] text-[hsl(var(--muted-foreground))]">
        <span>{series[0]?.date}</span>
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-1"><i className="h-2 w-2 rounded-sm bg-emerald-500/70" />上传</span>
          <span className="flex items-center gap-1"><i className="h-2 w-2 rounded-sm bg-blue-500/70" />下载</span>
        </div>
        <span>{series[series.length - 1]?.date}</span>
      </div>
    </div>
  );
}

export default function NodeDetailPage() {
  const { nodeId } = useParams({ strict: false }) as { nodeId: string };
  const navigate = useNavigate();
  const handleAuthError = useAuthErrorHandler();

  const [node, setNode] = useState<Node | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [inbounds, setInbounds] = useState<Inbound[]>([]);
  const [hostsByInbound, setHostsByInbound] = useState<Record<string, Host[]>>({});

  const [traffic, setTraffic] = useState<TrafficPoint[]>([]);
  const [days] = useState(30);

  const [sni, setSni] = useState<SNIStatusResponse | null>(null);
  const [sniError, setSniError] = useState<string | null>(null);

  const [checks, setChecks] = useState<CheckResultItem[]>([]);
  const [speedtest, setSpeedtest] = useState<SpeedTestResultItem | null>(null);
  const [traces, setTraces] = useState<TracerouteSnapshotItem[]>([]);

  const [speedRunning, setSpeedRunning] = useState(false);
  const [checkRunning, setCheckRunning] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const n = await api.get<Node>(`/nodes/${nodeId}`);
      setNode(n);
    } catch (err) {
      if (handleAuthError(err)) return;
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
      return;
    }

    try {
      const [ibResp, trafficResp] = await Promise.all([
        api.get<{ inbounds: Inbound[] }>(`/inbounds?node_id=${nodeId}`),
        api.get<TrafficResponse>(`/nodes/${nodeId}/traffic?days=${days}`),
      ]);
      setInbounds(ibResp.inbounds ?? []);
      setTraffic(trafficResp.points ?? []);

      // hosts per inbound
      const hostMap: Record<string, Host[]> = {};
      await Promise.all(
        (ibResp.inbounds ?? []).map(async (ib) => {
          try {
            const hr = await api.get<{ hosts: Host[] }>(`/inbounds/${ib.id}/hosts`);
            hostMap[ib.id] = hr.hosts ?? [];
          } catch {
            hostMap[ib.id] = [];
          }
        }),
      );
      setHostsByInbound(hostMap);
    } catch (err) {
      if (handleAuthError(err)) return;
      toast(err instanceof Error ? err.message : "加载入站/流量失败", "error");
    }

    try {
      const s = await api.get<SNIStatusResponse>(`/nodes/${nodeId}/sniproxy/status`);
      setSni(s);
      setSniError(null);
    } catch (err) {
      if (handleAuthError(err)) return;
      setSniError(err instanceof Error ? err.message : String(err));
    }

    try {
      const [checksResp, speedResp, traceResp] = await Promise.all([
        api.get<{ results: CheckResultItem[] }>(`/nodes/${nodeId}/checks`),
        api.get<SpeedTestResultItem>(`/nodes/${nodeId}/speedtest`),
        api.get<{ snapshots: TracerouteSnapshotItem[] }>(`/nodes/${nodeId}/traceroute/results?limit=20`),
      ]);
      setChecks(checksResp.results ?? []);
      setSpeedtest(speedResp && (speedResp.down_bps || speedResp.up_bps) ? speedResp : null);
      setTraces(traceResp.snapshots ?? []);
    } catch (err) {
      if (handleAuthError(err)) return;
      // 这些接口失败不阻塞主页面，只 toast 一次
      // toast(err instanceof Error ? err.message : "加载检测/测速失败", "error");
    }
    setLoading(false);
  }, [nodeId, days, handleAuthError]);

  useEffect(() => {
    reload();
  }, [reload]);

  if (loading && !node) {
    return <div className="p-6 text-sm text-[hsl(var(--muted-foreground))]">加载中…</div>;
  }
  if (error) {
    return (
      <div className="space-y-3 p-6">
        <p className="text-sm text-[hsl(var(--destructive))]">加载失败：{error}</p>
        <Button variant="outline" size="sm" onClick={() => navigate({ to: "/panel/nodes" })}>
          返回节点列表
        </Button>
      </div>
    );
  }
  if (!node) {
    return <div className="p-6 text-sm">节点不存在</div>;
  }

  const totalBytes = (node.upload_bytes ?? 0) + (node.download_bytes ?? 0);

  return (
    <div className="mx-auto max-w-6xl space-y-4 p-6">
      {/* Header */}
      <div className="sticky top-0 z-20 -mx-6 -mt-6 mb-4 flex flex-wrap items-center gap-3 border-b bg-[hsl(var(--background))]/95 px-6 py-3 backdrop-blur supports-[backdrop-filter]:bg-[hsl(var(--background))]/80">
        <Button variant="outline" size="sm" asChild>
          <Link to="/panel/nodes">← 返回</Link>
        </Button>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-2xl font-semibold">{node.name}</h1>
          <p className="truncate font-mono text-xs text-[hsl(var(--muted-foreground))]">{node.base_url}</p>
        </div>
        {node.disabled ? (
          <Badge variant="secondary">已禁用</Badge>
        ) : node.online ? (
          <Badge className="bg-emerald-500/15 text-emerald-700">在线</Badge>
        ) : (
          <Badge variant="secondary">离线</Badge>
        )}
        <Button variant="outline" size="sm" onClick={reload}>刷新</Button>
        <Button
          variant="outline"
          size="sm"
          onClick={async () => {
            try {
              await api.post(`/nodes/${nodeId}/runtime/restart`, {});
              toast("重启指令已发送", "success");
              setTimeout(reload, 1500);
            } catch (err) {
              if (!handleAuthError(err)) toast(err instanceof Error ? err.message : "重启失败", "error");
            }
          }}
        >重启</Button>
        <Button
          variant="outline"
          size="sm"
          disabled={speedRunning}
          onClick={async () => {
            setSpeedRunning(true);
            try {
              const res = await api.post<SpeedTestResultItem>(`/nodes/${nodeId}/runtime/speedtest`, {});
              setSpeedtest(res);
              toast("测速完成", "success");
            } catch (err) {
              if (!handleAuthError(err)) toast(err instanceof Error ? err.message : "测速失败", "error");
            } finally {
              setSpeedRunning(false);
            }
          }}
        >{speedRunning ? "测速中…" : "发起测速"}</Button>
        <Button
          variant="outline"
          size="sm"
          disabled={checkRunning}
          onClick={async () => {
            setCheckRunning(true);
            try {
              await api.post(`/nodes/${nodeId}/runtime/check`, {});
              toast("解锁检测完成", "success");
              const checksResp = await api.get<{ results: CheckResultItem[] }>(`/nodes/${nodeId}/checks`);
              setChecks(checksResp.results ?? []);
            } catch (err) {
              if (!handleAuthError(err)) toast(err instanceof Error ? err.message : "检测失败", "error");
            } finally {
              setCheckRunning(false);
            }
          }}
        >{checkRunning ? "检测中…" : "发起检测"}</Button>
      </div>

      {/* 基础信息 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">基础信息</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm md:grid-cols-3">
          <Field label="节点 ID" value={<span className="font-mono text-xs">{node.id}</span>} />
          <Field label="备注" value={node.remark || "—"} />
          <Field label="过期时间" value={fmtDate(node.expire_at)} />
          <Field label="IP 覆盖" value={node.ip_override || "—"} />
          <Field label="HTTPS 端口" value={node.https_port || "—"} />
          <Field label="是否落地" value={node.is_landing ? "是" : "否"} />
          <Field label="累计上传" value={formatBytes(node.upload_bytes ?? 0)} />
          <Field label="累计下载" value={formatBytes(node.download_bytes ?? 0)} />
          <Field label="累计合计" value={formatBytes(totalBytes)} />
          <Field
            label="面板 URL"
            value={node.panel_url ? <a href={node.panel_url} target="_blank" rel="noreferrer" className="break-all text-blue-600 hover:underline">{node.panel_url}</a> : "—"}
          />
          <Field label="ACME 邮箱" value={node.acme_email || "—"} />
          <Field label="面板域名" value={node.panel_domain || "—"} />
        </CardContent>
      </Card>

      {/* 流量趋势 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">流量趋势（{days} 天）</CardTitle>
          <CardDescription className="text-xs">来源：node_daily_usage（每分钟由 SyncUsage 任务累加）</CardDescription>
        </CardHeader>
        <CardContent>
          <TrafficChart points={traffic} days={days} />
        </CardContent>
      </Card>

      {/* 入站 + Host */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">入站 / 连接地址</CardTitle>
          <CardDescription className="text-xs">本节点上的所有入站及其 host 配置</CardDescription>
        </CardHeader>
        <CardContent>
          {inbounds.length === 0 ? (
            <p className="py-4 text-center text-xs text-[hsl(var(--muted-foreground))]">该节点没有入站</p>
          ) : (
            <div className="space-y-3">
              {inbounds.map((ib) => {
                const hosts = hostsByInbound[ib.id] ?? [];
                return (
                  <div key={ib.id} className="rounded-md border">
                    <div className="flex flex-wrap items-center gap-2 border-b bg-[hsl(var(--muted))/0.3] px-3 py-2 text-sm">
                      <span className="font-semibold">{ib.tag || ib.id}</span>
                      <Badge variant="outline" className="text-xs">{ib.protocol}</Badge>
                      <span className="text-xs text-[hsl(var(--muted-foreground))]">:{ib.port}</span>
                      {ib.traffic_rate !== 1 && (
                        <Badge variant="outline" className="text-xs">{ib.traffic_rate}x</Badge>
                      )}
                      <span className="ml-auto text-xs text-[hsl(var(--muted-foreground))]">{hosts.length} 个连接地址</span>
                    </div>
                    {hosts.length > 0 && (
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead className="text-xs">订阅名</TableHead>
                            <TableHead className="text-xs">地址</TableHead>
                            <TableHead className="text-xs">端口</TableHead>
                            <TableHead className="text-xs">前置节点</TableHead>
                            <TableHead className="text-xs">SNI</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {hosts.map((h) => (
                            <TableRow key={h.id}>
                              <TableCell className="text-xs">{h.remark || "—"}</TableCell>
                              <TableCell className="font-mono text-xs">{h.address}</TableCell>
                              <TableCell className="text-xs">{h.port || ib.port}</TableCell>
                              <TableCell className="text-xs">{h.relay_node_id ? `${h.relay_node_id.slice(0, 8)}…` : "—"}</TableCell>
                              <TableCell className="text-xs">{h.sni || "—"}</TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* NodeGate */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">NodeGate</CardTitle>
          <CardDescription className="text-xs">
            内置 SNI 代理（监听端口、路由表、证书）
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {sniError ? (
            <p className="text-xs text-[hsl(var(--destructive))]">读取失败：{sniError}</p>
          ) : !sni ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">加载中…</p>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm md:grid-cols-3">
                <Field
                  label="状态"
                  value={
                    sni.enabled
                      ? <Badge className="bg-emerald-500/15 text-emerald-700">监听 {sni.status.listen}</Badge>
                      : <Badge variant="secondary">未启用</Badge>
                  }
                />
                <Field label="路由数" value={sni.status.route_count} />
                <Field label="证书域名" value={sni.status.cert_domains} />
              </div>
              {sni.status.last_error && (
                <p className="text-xs text-[hsl(var(--destructive))]">最近错误：{sni.status.last_error}</p>
              )}
              {sni.status.routes && sni.status.routes.length > 0 && (
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="text-xs">SNI</TableHead>
                        <TableHead className="text-xs">后端</TableHead>
                        <TableHead className="text-xs">模式</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {sni.status.routes.map((r, i) => (
                        <TableRow key={i}>
                          <TableCell className="font-mono text-xs">{r.sni}</TableCell>
                          <TableCell className="font-mono text-xs">{r.backend}</TableCell>
                          <TableCell className="text-xs">{r.mode}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
              {sni.status.certs && sni.status.certs.length > 0 && (
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="text-xs">证书域名</TableHead>
                        <TableHead className="text-xs">状态</TableHead>
                        <TableHead className="text-xs">有效期</TableHead>
                        <TableHead className="text-xs">颁发者</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {sni.status.certs.map((c, i) => (
                        <TableRow key={i}>
                          <TableCell className="font-mono text-xs">{c.domain}</TableCell>
                          <TableCell className="text-xs">
                            {c.ready
                              ? <Badge className="bg-emerald-500/15 text-emerald-700">就绪</Badge>
                              : <Badge variant="secondary">未就绪</Badge>}
                          </TableCell>
                          <TableCell className="text-xs">{fmtDate(c.not_before)} → {fmtDate(c.not_after)}</TableCell>
                          <TableCell className="text-xs">{c.issuer || "—"}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* 测速 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">最近测速</CardTitle>
        </CardHeader>
        <CardContent>
          {!speedtest ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">暂无测速结果</p>
          ) : (
            <div className="grid grid-cols-3 gap-x-6 text-sm">
              <Field label="下载" value={speedtest.down_bps ? `${formatBytes(speedtest.down_bps)}/s` : "—"} />
              <Field label="上传" value={speedtest.up_bps ? `${formatBytes(speedtest.up_bps)}/s` : "—"} />
              <Field label="时间" value={fmtDate(speedtest.tested_at)} />
            </div>
          )}
        </CardContent>
      </Card>

      {/* 解锁检测 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">解锁检测</CardTitle>
          <CardDescription className="text-xs">最近一次检测的结果（直连 / 代理）</CardDescription>
        </CardHeader>
        <CardContent>
          {checks.length === 0 ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">尚未运行过检测</p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">服务</TableHead>
                    <TableHead className="text-xs">类型</TableHead>
                    <TableHead className="text-xs">状态</TableHead>
                    <TableHead className="text-xs">区域</TableHead>
                    <TableHead className="text-xs">备注</TableHead>
                    <TableHead className="text-xs">时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {checks.map((c, i) => (
                    <TableRow key={i}>
                      <TableCell className="text-xs">{c.service}</TableCell>
                      <TableCell className="text-xs">{c.check_type}</TableCell>
                      <TableCell className="text-xs">
                        {c.unlocked
                          ? <Badge className="bg-emerald-500/15 text-emerald-700">解锁</Badge>
                          : <Badge variant="secondary">未解锁</Badge>}
                      </TableCell>
                      <TableCell className="text-xs">{c.region || "—"}</TableCell>
                      <TableCell className="text-xs">{c.note || "—"}</TableCell>
                      <TableCell className="text-xs">{fmtDate(c.checked_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* 路由追踪 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">路由追踪</CardTitle>
          <CardDescription className="text-xs">最近 {traces.length} 条快照</CardDescription>
        </CardHeader>
        <CardContent>
          {traces.length === 0 ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">暂无追踪记录</p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">方向</TableHead>
                    <TableHead className="text-xs">目标</TableHead>
                    <TableHead className="text-xs">质量</TableHead>
                    <TableHead className="text-xs">时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {traces.map((t) => (
                    <TableRow key={t.id}>
                      <TableCell className="text-xs">{t.direction}</TableCell>
                      <TableCell className="text-xs">{t.target}</TableCell>
                      <TableCell className="text-xs">{t.quality || "—"}</TableCell>
                      <TableCell className="text-xs">{fmtDate(t.created_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-[hsl(var(--muted-foreground))]">{label}</div>
      <div className="truncate text-sm">{value}</div>
    </div>
  );
}
