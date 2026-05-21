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
import { useTranslation } from "react-i18next";
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

function TrafficChart({ points, days, t }: { points: TrafficPoint[]; days: number; t: (key: string, options?: Record<string, unknown>) => string }) {
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
        <span>{t("common.total")} <span className="font-semibold text-foreground">{formatBytes(grandTotal)}</span></span>
        <span>{t("common.upload")} <span className="font-semibold text-foreground">{formatBytes(grandUp)}</span></span>
        <span>{t("common.download")} <span className="font-semibold text-foreground">{formatBytes(grandDown)}</span></span>
        <span className="ml-auto">{t("nodeDetail.totalDays", { days })}</span>
      </div>
      <div className="flex h-40 items-end gap-px overflow-hidden rounded border bg-[hsl(var(--muted))/0.3] p-2">
        {series.map((s) => {
          const upPct = (s.up / max) * 100;
          const downPct = (s.down / max) * 100;
          return (
            <div
              key={s.date}
              className="group relative flex h-full flex-1 flex-col justify-end"
              title={`${s.date}\n${t("common.upload")}: ${formatBytes(s.up)}\n${t("common.download")}: ${formatBytes(s.down)}\n${t("common.total")}: ${formatBytes(s.total)}`}
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
          <span className="flex items-center gap-1"><i className="h-2 w-2 rounded-sm bg-emerald-500/70" />{t("common.upload")}</span>
          <span className="flex items-center gap-1"><i className="h-2 w-2 rounded-sm bg-blue-500/70" />{t("common.download")}</span>
        </div>
        <span>{series[series.length - 1]?.date}</span>
      </div>
    </div>
  );
}

export default function NodeDetailPage() {
  const { t } = useTranslation();
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
      toast(err instanceof Error ? err.message : t("nodeDetail.loadInboundFailed"), "error");
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
    return <div className="p-6 text-sm text-[hsl(var(--muted-foreground))]">{t("common.loading")}</div>;
  }
  if (error) {
    return (
      <div className="space-y-3 p-6">
        <p className="text-sm text-[hsl(var(--destructive))]">{t("nodeDetail.loadFailed", { error })}</p>
        <Button variant="outline" size="sm" onClick={() => navigate({ to: "/panel/nodes" })}>
          {t("nodeDetail.backToNodes")}
        </Button>
      </div>
    );
  }
  if (!node) {
    return <div className="p-6 text-sm">{t("nodeDetail.notFound")}</div>;
  }

  const totalBytes = (node.upload_bytes ?? 0) + (node.download_bytes ?? 0);

  return (
    <div className="mx-auto max-w-6xl space-y-4 p-6">
      {/* Header */}
      <div className="sticky top-0 z-20 -mx-6 -mt-6 mb-4 flex flex-wrap items-center gap-3 border-b bg-[hsl(var(--background))]/95 px-6 py-3 backdrop-blur supports-[backdrop-filter]:bg-[hsl(var(--background))]/80">
        <Button variant="outline" size="sm" asChild>
          <Link to="/panel/nodes">{t("nodeDetail.goBack")}</Link>
        </Button>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-2xl font-semibold">{node.name}</h1>
          <p className="truncate font-mono text-xs text-[hsl(var(--muted-foreground))]">{node.base_url}</p>
        </div>
        {node.disabled ? (
          <Badge variant="secondary">{t("common.disabled")}</Badge>
        ) : node.online ? (
          <Badge className="bg-emerald-500/15 text-emerald-700">{t("common.online")}</Badge>
        ) : (
          <Badge variant="secondary">{t("common.offline")}</Badge>
        )}
        <Button variant="outline" size="sm" onClick={reload}>{t("nodeDetail.refresh")}</Button>
        <Button
          variant="outline"
          size="sm"
          onClick={async () => {
            try {
              await api.post(`/nodes/${nodeId}/runtime/restart`, {});
              toast(t("nodeDetail.restartSent"), "success");
              setTimeout(reload, 1500);
            } catch (err) {
              if (!handleAuthError(err)) toast(err instanceof Error ? err.message : t("nodeDetail.restartFailed"), "error");
            }
          }}
        >{t("nodeDetail.restart")}</Button>
        <Button
          variant="outline"
          size="sm"
          disabled={speedRunning}
          onClick={async () => {
            setSpeedRunning(true);
            try {
              const res = await api.post<SpeedTestResultItem>(`/nodes/${nodeId}/runtime/speedtest`, {});
              setSpeedtest(res);
              toast(t("nodeDetail.speedtestDone"), "success");
            } catch (err) {
              if (!handleAuthError(err)) toast(err instanceof Error ? err.message : t("nodeDetail.speedtestFailed"), "error");
            } finally {
              setSpeedRunning(false);
            }
          }}
        >{speedRunning ? t("nodeDetail.speedtesting") : t("nodeDetail.startSpeedtest")}</Button>
        <Button
          variant="outline"
          size="sm"
          disabled={checkRunning}
          onClick={async () => {
            setCheckRunning(true);
            try {
              await api.post(`/nodes/${nodeId}/runtime/check`, {});
              toast(t("nodeDetail.checkDone"), "success");
              const checksResp = await api.get<{ results: CheckResultItem[] }>(`/nodes/${nodeId}/checks`);
              setChecks(checksResp.results ?? []);
            } catch (err) {
              if (!handleAuthError(err)) toast(err instanceof Error ? err.message : t("nodeDetail.checkFailed"), "error");
            } finally {
              setCheckRunning(false);
            }
          }}
        >{checkRunning ? t("nodeDetail.checking") : t("nodeDetail.startCheck")}</Button>
      </div>

      {/* 基础信息 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t("nodeDetail.basicInfo")}</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm md:grid-cols-3">
          <Field label={t("nodeDetail.nodeId")} value={<span className="font-mono text-xs">{node.id}</span>} />
          <Field label={t("nodeDetail.remark")} value={node.remark || "—"} />
          <Field label={t("nodeDetail.expireTime")} value={fmtDate(node.expire_at)} />
          <Field label={t("nodeDetail.ipOverride")} value={node.ip_override || "—"} />
          <Field label={t("nodeDetail.httpsPort")} value={node.https_port || "—"} />
          <Field label={t("nodeDetail.isLanding")} value={node.is_landing ? t("common.yes") : t("common.no")} />
          <Field label={t("nodeDetail.totalUpload")} value={formatBytes(node.upload_bytes ?? 0)} />
          <Field label={t("nodeDetail.totalDownload")} value={formatBytes(node.download_bytes ?? 0)} />
          <Field label={t("nodeDetail.totalAll")} value={formatBytes(totalBytes)} />
          <Field
            label={t("nodeDetail.panelURL")}
            value={node.panel_url ? <a href={node.panel_url} target="_blank" rel="noreferrer" className="break-all text-blue-600 hover:underline">{node.panel_url}</a> : "—"}
          />
          <Field label={t("nodeDetail.acmeEmail")} value={node.acme_email || "—"} />
          <Field label={t("nodeDetail.panelDomain")} value={node.panel_domain || "—"} />
        </CardContent>
      </Card>

      {/* 流量趋势 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t("nodeDetail.trafficTrend", { days })}</CardTitle>
          <CardDescription className="text-xs">{t("nodeDetail.trafficSource")}</CardDescription>
        </CardHeader>
        <CardContent>
          <TrafficChart points={traffic} days={days} t={t} />
        </CardContent>
      </Card>

      {/* 入站 + Host */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t("nodeDetail.inboundHosts")}</CardTitle>
          <CardDescription className="text-xs">{t("nodeDetail.inboundHostsDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          {inbounds.length === 0 ? (
            <p className="py-4 text-center text-xs text-[hsl(var(--muted-foreground))]">{t("nodeDetail.noInbounds")}</p>
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
                      <span className="ml-auto text-xs text-[hsl(var(--muted-foreground))]">{t("nodeDetail.hostsCount", { count: hosts.length })}</span>
                    </div>
                    {hosts.length > 0 && (
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead className="text-xs">{t("nodeDetail.subName")}</TableHead>
                            <TableHead className="text-xs">{t("common.address")}</TableHead>
                            <TableHead className="text-xs">{t("common.port")}</TableHead>
                            <TableHead className="text-xs">{t("nodeDetail.relayNode")}</TableHead>
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
          <CardTitle className="text-sm">{t("nodeDetail.nodeGate")}</CardTitle>
          <CardDescription className="text-xs">
            {t("nodeDetail.nodeGateDesc")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {sniError ? (
            <p className="text-xs text-[hsl(var(--destructive))]">{t("nodeDetail.readFailed", { error: sniError })}</p>
          ) : !sni ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("common.loading")}</p>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm md:grid-cols-3">
                <Field
                  label={t("common.status")}
                  value={
                    sni.enabled
                      ? <Badge className="bg-emerald-500/15 text-emerald-700">{t("nodeDetail.listen", { port: sni.status.listen })}</Badge>
                      : <Badge variant="secondary">{t("nodeDetail.notEnabled")}</Badge>
                  }
                />
                <Field label={t("nodeDetail.routeCount")} value={sni.status.route_count} />
                <Field label={t("nodeDetail.certDomain")} value={sni.status.cert_domains} />
              </div>
              {sni.status.last_error && (
                <p className="text-xs text-[hsl(var(--destructive))]">{t("nodeDetail.lastError", { error: sni.status.last_error })}</p>
              )}
              {sni.status.routes && sni.status.routes.length > 0 && (
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="text-xs">{t("nodeDetail.sni")}</TableHead>
                        <TableHead className="text-xs">{t("nodeDetail.backend")}</TableHead>
                        <TableHead className="text-xs">{t("nodeDetail.mode")}</TableHead>
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
                        <TableHead className="text-xs">{t("nodeDetail.certDomain")}</TableHead>
                        <TableHead className="text-xs">{t("nodeDetail.certStatus")}</TableHead>
                        <TableHead className="text-xs">{t("nodeDetail.validity")}</TableHead>
                        <TableHead className="text-xs">{t("nodeDetail.issuer")}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {sni.status.certs.map((c, i) => (
                        <TableRow key={i}>
                          <TableCell className="font-mono text-xs">{c.domain}</TableCell>
                          <TableCell className="text-xs">
                            {c.ready
                              ? <Badge className="bg-emerald-500/15 text-emerald-700">{t("nodeDetail.ready")}</Badge>
                              : <Badge variant="secondary">{t("nodeDetail.notReady")}</Badge>}
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
          <CardTitle className="text-sm">{t("nodeDetail.recentSpeedtest")}</CardTitle>
        </CardHeader>
        <CardContent>
          {!speedtest ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("nodeDetail.noSpeedtest")}</p>
          ) : (
            <div className="grid grid-cols-3 gap-x-6 text-sm">
              <Field label={t("nodeDetail.downloadSpeed")} value={speedtest.down_bps ? `${formatBytes(speedtest.down_bps)}/s` : "—"} />
              <Field label={t("nodeDetail.uploadSpeed")} value={speedtest.up_bps ? `${formatBytes(speedtest.up_bps)}/s` : "—"} />
              <Field label={t("nodeDetail.testTime")} value={fmtDate(speedtest.tested_at)} />
            </div>
          )}
        </CardContent>
      </Card>

      {/* 解锁检测 */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t("nodeDetail.unlockCheck")}</CardTitle>
          <CardDescription className="text-xs">{t("nodeDetail.unlockDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          {checks.length === 0 ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("nodeDetail.neverChecked")}</p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">{t("nodeDetail.service")}</TableHead>
                    <TableHead className="text-xs">{t("nodeDetail.type")}</TableHead>
                    <TableHead className="text-xs">{t("common.status")}</TableHead>
                    <TableHead className="text-xs">{t("nodeDetail.region")}</TableHead>
                    <TableHead className="text-xs">{t("common.remark")}</TableHead>
                    <TableHead className="text-xs">{t("nodeDetail.testTime")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {checks.map((c, i) => (
                    <TableRow key={i}>
                      <TableCell className="text-xs">{c.service}</TableCell>
                      <TableCell className="text-xs">{c.check_type}</TableCell>
                      <TableCell className="text-xs">
                        {c.unlocked
                          ? <Badge className="bg-emerald-500/15 text-emerald-700">{t("nodeDetail.unlocked")}</Badge>
                          : <Badge variant="secondary">{t("nodeDetail.locked")}</Badge>}
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
          <CardTitle className="text-sm">{t("nodeDetail.traceroute")}</CardTitle>
          <CardDescription className="text-xs">{t("nodeDetail.recentTraces", { count: traces.length })}</CardDescription>
        </CardHeader>
        <CardContent>
          {traces.length === 0 ? (
            <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("nodeDetail.noTraces")}</p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">{t("nodeDetail.direction")}</TableHead>
                    <TableHead className="text-xs">{t("nodeDetail.target")}</TableHead>
                    <TableHead className="text-xs">{t("nodeDetail.quality")}</TableHead>
                    <TableHead className="text-xs">{t("nodeDetail.testTime")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {traces.map((tr) => (
                    <TableRow key={tr.id}>
                      <TableCell className="text-xs">{tr.direction}</TableCell>
                      <TableCell className="text-xs">{tr.target}</TableCell>
                      <TableCell className="text-xs">{tr.quality || "—"}</TableCell>
                      <TableCell className="text-xs">{fmtDate(tr.created_at)}</TableCell>
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
