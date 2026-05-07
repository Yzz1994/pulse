import { useState, useEffect, useCallback, useRef } from "react";
import { Card, CardContent, CardHeader } from "@/components/ui";
import { getTheme, toggleTheme, type Theme } from "@/lib/theme";
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid,
  Tooltip as RechartsTooltip, ResponsiveContainer, Legend,
} from "recharts";

/* ── Types ────────────────────────────────────────────────────────── */

interface TracerouteSnapshotItem {
  id: string;
  direction: "inbound" | "outbound";
  target: string;
  hops: string;   // JSON 字符串，需要 JSON.parse
  quality: string;
  created_at: string;
}

interface HopGeo {
  country_code?: string;
  country_name?: string;
  city?: string;
  asn_org?: string;
}

interface TracerouteHop {
  hop: number;
  ip?: string;
  rtt_ms?: number[];
  timeout?: boolean;
  geo?: HopGeo;
  network?: string; // "CN2" / "163" / "CU" 等
}

interface Check {
  service: string;
  unlocked: boolean;
  region: string;
  note: string;
  checked_at?: string;
}

interface SpeedTest {
  down_bps: number;
  up_bps: number;
  tested_at: string;
}

interface UptimeBar {
  label: string;
  online_pct: number;
}

interface NodeGeo {
  country_code?: string;
  country_name?: string;
  city?: string;
  asn_org?: string;
}

interface Node {
  id: string;
  name: string;
  geo?: NodeGeo | null;
  direct_checks: Check[];
  proxied_checks: Check[];
  speed_test: SpeedTest | null;
  uptime_bars: UptimeBar[];
  granularity: string;
  overall_pct: number;
  has_data: boolean;
  checked_at: string;
  unlocked_count: number;
  total_count: number;
  unlock_pct: number;
  traceroutes?: TracerouteSnapshotItem[];
  latency?: LatencySample[];
}

interface LatencySample {
  isp: "ct" | "cu" | "cm";
  rtt_ms: number | null;
  sampled_at: string;
}

interface NodeMetrics {
  node_id: string;
  upload_speed: number;
  download_speed: number;
  connections: number;
  running: boolean;
}

interface StatData {
  nodes: Node[];
  node_count: number;
  unlock_rate: number;
  avg_uptime_pct: number;
  service_count: number;
  updated_at: string;
}

/* ── Helpers ──────────────────────────────────────────────────────── */

function qualityColor(q: string): string {
  if (q === "CN2 GIA" || q === "联通 AS9929") return "text-emerald-600";
  if (q === "CN2 GT") return "text-blue-600";
  return "text-amber-600";
}

function isPrivateIP(ip: string): boolean {
  if (ip.startsWith("10.") || ip.startsWith("127.") || ip === "::1") return true;
  if (ip.startsWith("192.168.")) return true;
  if (ip.startsWith("100.")) {
    const second = parseInt(ip.split(".")[1] ?? "0");
    if (second >= 64 && second <= 127) return true;
  }
  if (ip.startsWith("11.")) return true;
  const parts = ip.split(".").map(Number);
  if (parts.length === 4 && (parts[0] ?? 0) === 172 && (parts[1] ?? 0) >= 16 && (parts[1] ?? 0) <= 31) return true;
  return false;
}

function formatSpeed(bps: number): string {
  if (bps >= 1_073_741_824) return `${(bps / 1_073_741_824).toFixed(2)} GB/s`;
  if (bps >= 1_048_576) return `${(bps / 1_048_576).toFixed(1)} MB/s`;
  if (bps >= 1024) return `${Math.round(bps / 1024)} KB/s`;
  return `${bps} B/s`;
}

function formatTime(iso: string | undefined): string {
  if (!iso) return "—";
  // 确保 UTC 字符串末尾有 Z，避免被解析成本地时间
  const normalized = iso.endsWith("Z") || iso.includes("+") ? iso : iso + "Z";
  const d = new Date(normalized);
  if (isNaN(d.getTime())) return "—";
  return d.toLocaleString("zh-CN", {
    month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", hour12: false,
  });
}

function formatBarLabel(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  return `${mm}/${dd} ${hh}:00`;
}

function uptimeBarStyle(pct: number): React.CSSProperties {
  if (pct < 0) return { backgroundColor: "hsl(var(--muted))" };
  if (pct >= 95) return { backgroundColor: "#10b981" };    // emerald-500
  if (pct >= 80) return { backgroundColor: "#eab308" };    // yellow-500
  if (pct >= 50) return { backgroundColor: "#f97316" };    // orange-500
  return { backgroundColor: "#ef4444" };                    // red-500
}

// 对公网 IP 脱敏：保留前两段，后两段替换为 *
function maskIP(ip: string | undefined): string {
  if (!ip) return "—";
  const parts = ip.split(".");
  if (parts.length === 4) return `${parts[0]}.${parts[1]}.*.*`;
  // IPv6 保留前两组
  const v6 = ip.split(":");
  if (v6.length >= 2) return `${v6[0]}:${v6[1]}:…`;
  return ip;
}

function uptimeTextColor(pct: number): string {
  if (pct < 0) return "text-zinc-500";
  if (pct >= 99) return "text-emerald-400";
  if (pct >= 95) return "text-yellow-400";
  return "text-red-400";
}

/* ── Skeleton ─────────────────────────────────────────────────────── */

function Skeleton({ className = "" }: { className?: string }) {
  return (
    <div
      className={`animate-pulse rounded-md bg-[hsl(var(--muted))] ${className}`}
    />
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-6">
      {/* Summary cards skeleton */}
      <div className="grid grid-cols-2 gap-4">
        {Array.from({ length: 2 }).map((_, i) => (
          <Card key={i}>
            <CardHeader className="pb-2 pt-4 px-4">
              <Skeleton className="h-3.5 w-16" />
            </CardHeader>
            <CardContent className="px-4 pb-4">
              <Skeleton className="h-8 w-20" />
            </CardContent>
          </Card>
        ))}
      </div>
      {/* Node cards skeleton */}
      {Array.from({ length: 3 }).map((_, i) => (
        <Card key={i}>
          <CardHeader className="pb-3">
            <div className="flex items-center gap-3">
              <Skeleton className="h-3 w-3 rounded-full" />
              <Skeleton className="h-5 w-32" />
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-16 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

/* ── Summary Stat Card ────────────────────────────────────────────── */

function SummaryCard({
  label,
  value,
  suffix,
  colorClass,
}: {
  label: string;
  value: string | number;
  suffix?: string;
  colorClass?: string;
}) {
  return (
    <Card>
      <CardHeader className="pb-1 pt-4 px-4">
        <span className="text-xs font-medium text-[hsl(var(--muted-foreground))]">
          {label}
        </span>
      </CardHeader>
      <CardContent className="px-4 pb-4">
        <span className={`text-2xl font-bold tabular-nums ${colorClass ?? "text-[hsl(var(--foreground))]"}`}>
          {value}
        </span>
        {suffix && (
          <span className={`ml-0.5 text-sm font-medium ${colorClass ?? "text-[hsl(var(--muted-foreground))]"}`}>
            {suffix}
          </span>
        )}
      </CardContent>
    </Card>
  );
}

/* ── Uptime Bar Chart ─────────────────────────────────────────────── */

function UptimeBars({ bars }: { bars: UptimeBar[] }) {
  if (!bars.length) return null;
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1">
        {bars.map((bar, i) => (
          <div key={i} className="group relative flex-1">
            <div
              className="h-6 rounded-sm transition-opacity hover:opacity-80"
              style={uptimeBarStyle(bar.online_pct)}
              title={`${formatBarLabel(bar.label)}: ${bar.online_pct < 0 ? "无数据" : `${bar.online_pct}%`}`}
            />
            {/* Tooltip on hover */}
            <div className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1.5 -translate-x-1/2 whitespace-nowrap rounded-md border border-[hsl(var(--border))] bg-zinc-900 px-2 py-1 text-xs text-zinc-300 opacity-0 shadow-lg transition-opacity group-hover:opacity-100">
              {formatBarLabel(bar.label)}
              {": "}
              {bar.online_pct < 0 ? "无数据" : `${bar.online_pct}%`}
            </div>
          </div>
        ))}
      </div>
      <div className="flex justify-between text-[11px] text-zinc-500">
        <span>3d前</span>
        <span>现在</span>
      </div>
    </div>
  );
}

/* ── Latency Sparkline ───────────────────────────────────────────── */

const SPARK_ISP_COLOR: Record<string, string> = { ct: "#ef4444", cu: "#3b82f6", cm: "#22c55e" };
const ISP_ORDER = ["ct", "cu", "cm"] as const;

function LatencySparkline({ samples }: { samples: LatencySample[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [hoverX, setHoverX] = useState<number | null>(null); // 0–100 viewBox units

  const valid = samples.filter((s) => s.rtt_ms !== null);
  if (!valid.length) return null;

  const byISP: Record<string, LatencySample[]> = {};
  samples.forEach((s) => {
    if (s.rtt_ms !== null) (byISP[s.isp] ??= []).push(s);
  });
  // 按时间排序
  Object.values(byISP).forEach((pts) => pts.sort((a, b) => a.sampled_at.localeCompare(b.sampled_at)));

  const allRtts = valid.map((s) => s.rtt_ms!);
  const minR = Math.min(...allRtts);
  const maxR = Math.max(...allRtts);
  const rRange = maxR - minR || 1;

  const times = samples.map((s) => new Date(s.sampled_at).getTime());
  const minT = Math.min(...times);
  const maxT = Math.max(...times);
  const tRange = maxT - minT || 1;

  const W = 100;
  const H = 32;
  const toX = (iso: string) => ((new Date(iso).getTime() - minT) / tRange) * W;
  const toY = (v: number) => H - 2 - ((v - minR) / rRange) * (H - 4);

  // 每个 ISP 的统计
  const stats = ISP_ORDER.filter((isp) => byISP[isp]).map((isp) => {
    const rtts = byISP[isp]!.map((s) => s.rtt_ms!);
    return {
      isp,
      min: Math.min(...rtts),
      max: Math.max(...rtts),
      avg: Math.round(rtts.reduce((a, b) => a + b, 0) / rtts.length),
      jitter: (() => {
        const avg = rtts.reduce((a, b) => a + b, 0) / rtts.length;
        return Math.round(Math.sqrt(rtts.reduce((a, b) => a + (b - avg) ** 2, 0) / rtts.length));
      })(),
    };
  });

  // hover: 找最近的时间点，返回各 ISP 的 rtt
  const hoverPoints = hoverX !== null
    ? ISP_ORDER.filter((isp) => byISP[isp]).map((isp) => {
        const pts = byISP[isp]!;
        const targetT = minT + (hoverX / W) * tRange;
        const nearest = pts.reduce((best, p) =>
          Math.abs(new Date(p.sampled_at).getTime() - targetT) <
          Math.abs(new Date(best.sampled_at).getTime() - targetT) ? p : best
        );
        return { isp, rtt: nearest.rtt_ms!, x: toX(nearest.sampled_at), y: toY(nearest.rtt_ms!), time: nearest.sampled_at };
      })
    : [];

  const hoverTime = hoverPoints[0]?.time ?? null;

  const handleMouseMove = (e: React.MouseEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    setHoverX(Math.max(0, Math.min(W, pct * W)));
  };

  return (
    <div ref={containerRef} className="space-y-1.5">
      {/* SVG sparkline */}
      <div className="relative">
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          className="w-full cursor-crosshair"
          style={{ height: H }}
          onMouseMove={handleMouseMove}
          onMouseLeave={() => setHoverX(null)}
        >
          {/* 折线 */}
          {Object.entries(byISP).map(([isp, pts]) => {
            const d = pts.map((p, i) =>
              `${i === 0 ? "M" : "L"}${toX(p.sampled_at).toFixed(1)},${toY(p.rtt_ms!).toFixed(1)}`
            ).join(" ");
            return <path key={isp} d={d} fill="none" stroke={SPARK_ISP_COLOR[isp] ?? "#888"} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />;
          })}
          {/* hover 十字线 + 圆点 */}
          {hoverX !== null && (
            <>
              <line x1={hoverX} x2={hoverX} y1={0} y2={H} stroke="#fff" strokeWidth="0.5" opacity="0.25" vectorEffect="non-scaling-stroke" />
              {hoverPoints.map((p) => (
                <circle key={p.isp} cx={p.x} cy={p.y} r="2" fill={SPARK_ISP_COLOR[p.isp] ?? "#888"} vectorEffect="non-scaling-stroke" />
              ))}
            </>
          )}
        </svg>
        {/* Tooltip */}
        {hoverX !== null && hoverPoints.length > 0 && (
          <div
            className="pointer-events-none absolute top-0 z-10 rounded-md border border-[hsl(var(--border))] bg-zinc-900/95 px-2 py-1.5 text-[11px] shadow-lg"
            style={{ left: hoverX > 60 ? "auto" : `calc(${hoverX}% + 6px)`, right: hoverX > 60 ? `calc(${100 - hoverX}% + 6px)` : "auto", top: 0 }}
          >
            {hoverTime && (
              <div className="mb-1 text-[10px] text-zinc-500 tabular-nums">
                {new Date(hoverTime.endsWith("Z") || hoverTime.includes("+") ? hoverTime : hoverTime + "Z")
                  .toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false })}
              </div>
            )}
            {hoverPoints.map((p) => (
              <div key={p.isp} className="flex items-center gap-2">
                <span className="inline-block h-1.5 w-2.5 rounded-full" style={{ background: SPARK_ISP_COLOR[p.isp] }} />
                <span className="text-zinc-400">{ISP_LABEL[p.isp] ?? p.isp}</span>
                <span className="ml-auto pl-3 font-medium tabular-nums text-zinc-100">{p.rtt} ms</span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 横轴时间范围 */}
      <div className="flex justify-between text-[10px] text-zinc-600 -mt-0.5">
        <span>{new Date(minT).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false })}</span>
        <span>{new Date(maxT).toLocaleString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false })}</span>
      </div>

      {/* ISP 统计表：每 ISP 一行，列对齐 */}
      <div className="text-[10px]">
        {/* 表头 */}
        <div className="mb-0.5 grid text-[9px] text-zinc-600" style={{ gridTemplateColumns: "1fr 3rem 3rem 3rem 3rem" }}>
          <span />
          <span className="text-right">min</span>
          <span className="text-right">avg</span>
          <span className="text-right">max</span>
          <span className="text-right">jitter</span>
        </div>
        {/* 数据行 */}
        {stats.map(({ isp, min, avg, max, jitter }) => (
          <div key={isp} className="grid items-center py-0.5" style={{ gridTemplateColumns: "1fr 3rem 3rem 3rem 3rem" }}>
            <span className="flex items-center gap-1.5 text-zinc-400">
              <span className="inline-block h-1.5 w-2.5 shrink-0 rounded-full" style={{ background: SPARK_ISP_COLOR[isp] }} />
              {ISP_LABEL[isp] ?? isp}
            </span>
            <span className="text-right tabular-nums text-zinc-500">{min}</span>
            <span className="text-right tabular-nums font-medium text-zinc-200">{avg}</span>
            <span className="text-right tabular-nums text-zinc-500">{max}</span>
            <span className="text-right tabular-nums text-amber-500/80">±{jitter}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/* ── Check Badges Row ─────────────────────────────────────────────── */

/* ── Traceroute Record ────────────────────────────────────────────── */

const NETWORK_COLOR: Record<string, string> = {
  CN2:  "text-emerald-500 bg-emerald-500/10",
  "163": "text-amber-500 bg-amber-500/10",
  CU:   "text-blue-500 bg-blue-500/10",
  CU2:  "text-emerald-500 bg-emerald-500/10",
  CMI:  "text-purple-500 bg-purple-500/10",
};

function TracerouteRecord({ item }: { item: TracerouteSnapshotItem }) {
  const [expanded, setExpanded] = useState(false);
  const dirLabel = item.direction === "inbound" ? "[回程]" : "[去程]";
  const timeStr = formatTime(item.created_at);
  const isGood = item.quality === "CN2 GIA" || item.quality === "联通 AS9929";

  const hops: TracerouteHop[] = (() => {
    try { return JSON.parse(item.hops); } catch { return []; }
  })();

  const avgRtt = (rtts?: number[]) => {
    if (!rtts?.length) return "—";
    return (rtts.reduce((a, b) => a + b, 0) / rtts.length).toFixed(1) + "ms";
  };

  return (
    <div className={`rounded-md border pt-2 px-2 pb-1.5 ${isGood ? "border-emerald-500/30 bg-emerald-500/5" : "border-[hsl(var(--border))]"}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 text-left text-xs hover:opacity-80"
      >
        <div className="flex flex-1 items-center gap-2 min-w-0">
          <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${item.direction === "inbound" ? "bg-blue-500/15 text-blue-400" : "bg-purple-500/15 text-purple-400"}`}>
            {item.direction === "inbound" ? "回程" : "去程"}
          </span>
          {item.quality && (
            <span className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold ${qualityColor(item.quality)}`}>
              {item.quality}
            </span>
          )}
          <span className="truncate text-zinc-400 font-mono text-[11px]">{maskIP(item.target)}</span>
        </div>
        <span className="shrink-0 text-zinc-500 text-[11px]">{timeStr}</span>
        <span className="shrink-0 text-zinc-600">{expanded ? "▲" : "▼"}</span>
      </button>
      {expanded && hops.length > 0 && (
        <div className="mt-2 space-y-0.5 font-mono text-[11px]">
          {hops.map((hop) => {
            const isPrivate = hop.ip && isPrivateIP(hop.ip);
            const flag = hop.geo?.country_code
              ? String.fromCodePoint(...[...hop.geo.country_code.toUpperCase()].map(c => 0x1F1E6 - 65 + c.charCodeAt(0)))
              : "";
            return (
              <div key={hop.hop} className="flex items-center gap-2">
                <span className="w-4 shrink-0 text-right text-zinc-500">{hop.hop}</span>
                <span className={`w-28 shrink-0 truncate ${hop.timeout ? "text-zinc-600" : "text-[hsl(var(--foreground))]"}`}>
                  {hop.timeout ? "* * *" : (isPrivate ? hop.ip : maskIP(hop.ip))}
                </span>
                {/* 网络类型标签 */}
                <span className="w-10 shrink-0">
                  {isPrivate ? (
                    <span className="text-zinc-600">内网</span>
                  ) : hop.network ? (
                    <span className={`rounded px-1 py-0.5 text-[10px] font-medium ${NETWORK_COLOR[hop.network] ?? "text-zinc-400"}`}>
                      {hop.network}
                    </span>
                  ) : null}
                </span>
                {/* 地区 */}
                <span className="flex-1 truncate text-zinc-500">
                  {!hop.timeout && !isPrivate && hop.geo && (
                    <>
                      {flag && <span className="mr-0.5">{flag}</span>}
                      {[hop.geo.country_name, hop.geo.city].filter(Boolean).join(" · ")}
                      {hop.geo.asn_org && <span className="ml-1 opacity-60">{hop.geo.asn_org}</span>}
                    </>
                  )}
                </span>
                <span className="shrink-0 text-zinc-400">
                  {hop.timeout ? "" : avgRtt(hop.rtt_ms)}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

const TRACEROUTE_DISPLAY_LIMIT = 3;

function NodeTraceroute({ snapshots }: { snapshots: TracerouteSnapshotItem[] }) {
  const [showAll, setShowAll] = useState(false);
  if (!snapshots || snapshots.length === 0) return null;
  const visible = showAll ? snapshots : snapshots.slice(0, TRACEROUTE_DISPLAY_LIMIT);
  const hidden = snapshots.length - TRACEROUTE_DISPLAY_LIMIT;
  return (
    <div className="space-y-2">
      <div className="text-[11px] font-medium text-zinc-400">路由追踪记录</div>
      {visible.map(item => (
        <TracerouteRecord key={item.id} item={item} />
      ))}
      {!showAll && hidden > 0 && (
        <button
          onClick={() => setShowAll(true)}
          className="text-[11px] text-zinc-500 hover:text-zinc-300 transition-colors"
        >
          还有 {hidden} 条 ▼
        </button>
      )}
    </div>
  );
}

/* ── Node Card ────────────────────────────────────────────────────── */

function countryFlag(code: string): string {
  if (!code || code.length !== 2) return "";
  return String.fromCodePoint(...[...code.toUpperCase()].map(c => 0x1F1E6 - 65 + c.charCodeAt(0)));
}

function speedStyle(cur: number, prev: number): React.CSSProperties {
  if (cur === 0) return { color: "hsl(var(--muted-foreground))" };
  if (cur > prev) return { color: "#10b981" }; // emerald-500
  if (cur < prev) return { color: "#fb923c" }; // orange-400
  return { color: "hsl(var(--foreground))" };
}

/* ── 延迟图表辅助 ─────────────────────────────────────────────────── */

const ISP_LABEL: Record<string, string> = { ct: "上海电信", cu: "上海联通", cm: "上海移动" };
const ISP_COLOR: Record<string, string> = { ct: "#ef4444", cu: "#3b82f6", cm: "#22c55e" };

function buildLatencyChartData(samples: LatencySample[]) {
  const buckets = new Map<string, Record<string, number[]>>();
  samples.forEach((s) => {
    const t = s.sampled_at.slice(0, 16); // 含日期避免跨天同时段合并
    if (!buckets.has(t)) buckets.set(t, {});
    const b = buckets.get(t)!;
    if (s.rtt_ms !== null) {
      (b[s.isp] ??= []).push(s.rtt_ms);
    }
  });
  return [...buckets.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([t, b]) => ({
      t: t.slice(11), // 显示时只展示 HH:MM
      ct: b.ct?.length ? Math.round(b.ct.reduce((a, v) => a + v, 0) / b.ct.length) : null,
      cu: b.cu?.length ? Math.round(b.cu.reduce((a, v) => a + v, 0) / b.cu.length) : null,
      cm: b.cm?.length ? Math.round(b.cm.reduce((a, v) => a + v, 0) / b.cm.length) : null,
    }));
}

/* ── 节点详情弹窗 ─────────────────────────────────────────────────── */

function NodeDetailDialog({ node, onClose }: { node: Node; onClose: () => void }) {
  const [expandedSnap, setExpandedSnap] = useState<string | null>(null);

  // ESC 关闭
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  const latencyData = buildLatencyChartData(node.latency ?? []);
  const hasLatency = latencyData.length > 0;
  const hasTraceroute = (node.traceroutes ?? []).length > 0;
  const hasChecks = (node.direct_checks?.length ?? 0) > 0 || (node.proxied_checks?.length ?? 0) > 0;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      <div
        className="relative w-full max-w-2xl max-h-[85vh] overflow-y-auto rounded-xl border border-[hsl(var(--border))] bg-[hsl(var(--background))] shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 标题栏 */}
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-[hsl(var(--border))] bg-[hsl(var(--background))] px-5 py-3.5">
          <div className="flex items-center gap-2.5">
            <span className="font-semibold text-[hsl(var(--foreground))]">{node.name}</span>
            {node.geo && (
              <span className="text-xs text-zinc-400">
                {[node.geo.country_name, node.geo.city].filter(Boolean).join(" · ")}
              </span>
            )}
          </div>
          <button
            onClick={onClose}
            className="rounded-md p-1 text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--foreground))] transition-colors"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"
              stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"
              className="h-4 w-4">
              <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <div className="space-y-5 p-5">
          {/* 解锁情况 */}
          {hasChecks && (
            <div>
              <p className="mb-2 text-xs font-semibold text-[hsl(var(--muted-foreground))] uppercase tracking-wide">解锁情况</p>
              <div className="space-y-3">
                {(["direct_checks", "proxied_checks"] as const).map((key) => {
                  const checks = node[key];
                  if (!checks?.length) return null;
                  const label = key === "direct_checks" ? "直连" : "代理";
                  return (
                    <div key={key}>
                      <p className="mb-1.5 text-[11px] font-medium text-zinc-500">{label}</p>
                      <div className="grid grid-cols-2 gap-1 sm:grid-cols-3">
                        {checks.map((c) => (
                          <div
                            key={c.service}
                            className={`flex items-center justify-between rounded-md px-2.5 py-1.5 text-xs ${
                              c.unlocked
                                ? "bg-emerald-500/10 text-emerald-400"
                                : "bg-[hsl(var(--muted))] text-zinc-500"
                            }`}
                          >
                            <span className="truncate font-medium">{c.service}</span>
                            <span className="ml-2 shrink-0 tabular-nums">
                              {c.unlocked ? (c.region || "✓") : "✗"}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* 延迟折线图 */}
          {hasLatency ? (
            <div>
              <p className="mb-2 text-xs font-semibold text-[hsl(var(--muted-foreground))] uppercase tracking-wide">上海三网延迟（过去 1h）</p>
              <ResponsiveContainer width="100%" height={200}>
                <LineChart data={latencyData} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" opacity={0.5} />
                  <XAxis dataKey="t" tick={{ fontSize: 10, fill: "hsl(var(--muted-foreground))" }} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                  <YAxis tick={{ fontSize: 10, fill: "hsl(var(--muted-foreground))" }} tickLine={false} axisLine={false} unit="ms" width={42} />
                  <RechartsTooltip
                    contentStyle={{ background: "hsl(var(--background))", border: "1px solid hsl(var(--border))", borderRadius: 8, fontSize: 12 }}
                    formatter={(v: unknown, name: unknown) => [`${v} ms`, ISP_LABEL[String(name)] ?? String(name)] as [string, string]}
                  />
                  <Legend formatter={(v) => <span style={{ fontSize: 11 }}>{ISP_LABEL[v] ?? v}</span>} />
                  {(["ct", "cu", "cm"] as const).map((isp) => (
                    <Line key={isp} type="monotone" dataKey={isp} stroke={ISP_COLOR[isp]} strokeWidth={1.5} dot={false} activeDot={{ r: 3 }} connectNulls={false} />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <div className="rounded-lg border border-[hsl(var(--border))] py-6 text-center text-xs text-[hsl(var(--muted-foreground))]">
              暂无延迟数据（每分钟采样，部署后约 1 分钟内出现）
            </div>
          )}

          {/* 路由追踪快照 */}
          {hasTraceroute && (
            <div>
              <p className="mb-2 text-xs font-semibold text-[hsl(var(--muted-foreground))] uppercase tracking-wide">路由追踪快照</p>
              <div className="space-y-2">
                {node.traceroutes!.map((snap, i) => {
                  const key = `${snap.direction}-${snap.target}-${i}`;
                  const expanded = expandedSnap === key;
                  let hops: TracerouteHop[] = [];
                  try { hops = JSON.parse(snap.hops); } catch {}
                  return (
                    <div key={key} className="rounded-lg border border-[hsl(var(--border))] overflow-hidden">
                      <button
                        className="w-full flex items-center justify-between px-3 py-2 text-xs hover:bg-[hsl(var(--accent))/0.4] transition-colors"
                        onClick={() => setExpandedSnap(expanded ? null : key)}
                      >
                        <span className="flex items-center gap-2">
                          <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${snap.direction === "inbound" ? "bg-blue-500/15 text-blue-400" : "bg-purple-500/15 text-purple-400"}`}>
                            {snap.direction === "inbound" ? "回程" : "去程"}
                          </span>
                          <span className="font-mono text-[hsl(var(--foreground))]">{snap.target}</span>
                          {snap.quality && (
                            <span className="text-[hsl(var(--muted-foreground))]">· {snap.quality}</span>
                          )}
                        </span>
                        <span className={`text-[hsl(var(--muted-foreground))] transition-transform ${expanded ? "rotate-180" : ""}`}>▾</span>
                      </button>
                      {expanded && hops.length > 0 && (
                        <div className="border-t border-[hsl(var(--border))]">
                          <table className="w-full text-[11px] font-mono">
                            <thead className="bg-[hsl(var(--muted))]">
                              <tr>
                                <th className="px-2 py-1 text-left text-[hsl(var(--muted-foreground))] font-medium">#</th>
                                <th className="px-2 py-1 text-left text-[hsl(var(--muted-foreground))] font-medium">IP</th>
                                <th className="px-2 py-1 text-left text-[hsl(var(--muted-foreground))] font-medium">网络</th>
                                <th className="px-2 py-1 text-right text-[hsl(var(--muted-foreground))] font-medium">RTT</th>
                              </tr>
                            </thead>
                            <tbody>
                              {hops.map((hop) => (
                                <tr key={hop.hop} className="border-t border-[hsl(var(--border))]">
                                  <td className="px-2 py-1 text-[hsl(var(--muted-foreground))]">{hop.hop}</td>
                                  <td className="px-2 py-1">
                                    {hop.timeout ? <span className="opacity-40">* * *</span> : (hop.ip && isPrivateIP(hop.ip) ? hop.ip : maskIP(hop.ip))}
                                  </td>
                                  <td className="px-2 py-1 text-[hsl(var(--muted-foreground))]">{hop.network ?? ""}</td>
                                  <td className="px-2 py-1 text-right">
                                    {hop.rtt_ms?.length
                                      ? `${(hop.rtt_ms.reduce((a, b) => a + b, 0) / hop.rtt_ms.length).toFixed(1)} ms`
                                      : <span className="opacity-40">—</span>}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {!hasLatency && !hasTraceroute && (
            <p className="text-center text-xs text-[hsl(var(--muted-foreground))] py-4">暂无详细数据</p>
          )}
        </div>
      </div>
    </div>
  );
}

function NodeCard({ node, metrics, prevMetrics }: { node: Node; metrics?: NodeMetrics; prevMetrics?: NodeMetrics }) {
  const hasSpeed = metrics && (metrics.upload_speed > 0 || metrics.download_speed > 0);
  const [dialogOpen, setDialogOpen] = useState(false);
  const closeDialog = useCallback(() => setDialogOpen(false), []);
  const hasDetail = (node.latency?.length ?? 0) > 0 || (node.traceroutes?.length ?? 0) > 0
    || (node.direct_checks?.length ?? 0) > 0 || (node.proxied_checks?.length ?? 0) > 0;
  return (
    <>
    {dialogOpen && <NodeDetailDialog node={node} onClose={closeDialog} />}
    <Card>
      {/* ── Header ── */}
      <CardHeader className="pb-3">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          {/* Status dot + name */}
          <div className="flex items-center gap-2.5 min-w-0">
            <span
              className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${
                (() => {
                  const lastBar = node.uptime_bars?.at(-1);
                  if (!lastBar || lastBar.online_pct < 0) return "bg-zinc-600";
                  if (lastBar.online_pct >= 95) return "bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.4)]";
                  if (lastBar.online_pct >= 80) return "bg-yellow-500 shadow-[0_0_6px_rgba(234,179,8,0.4)]";
                  if (lastBar.online_pct >= 50) return "bg-orange-500 shadow-[0_0_6px_rgba(249,115,22,0.4)]";
                  return "bg-red-500 shadow-[0_0_6px_rgba(239,68,68,0.4)]";
                })()
              }`}
            />
            <span className="text-sm font-semibold text-[hsl(var(--foreground))]">
              {node.name}
            </span>
            {node.geo && (
              <span className="text-xs text-zinc-400 truncate">
                {countryFlag(node.geo.country_code ?? "")}
                {" "}
                {[node.geo.country_name, node.geo.city].filter(Boolean).join(" · ")}
                {node.geo.asn_org && <span className="opacity-60"> / {node.geo.asn_org}</span>}
              </span>
            )}
          </div>

          {/* 实时网速 + 详情按钮 */}
          <div className="ml-auto flex items-center gap-3 shrink-0">
            {hasSpeed && (
              <div className="flex items-center gap-1.5 text-xs tabular-nums">
                <span style={speedStyle(metrics!.download_speed, prevMetrics?.download_speed ?? metrics!.download_speed)}>
                  ↓{formatSpeed(metrics!.download_speed)}
                </span>
                <span className="opacity-30">·</span>
                <span style={speedStyle(metrics!.upload_speed, prevMetrics?.upload_speed ?? metrics!.upload_speed)}>
                  ↑{formatSpeed(metrics!.upload_speed)}
                </span>
              </div>
            )}
            <button
              onClick={() => hasDetail && setDialogOpen(true)}
              disabled={!hasDetail}
              className="flex items-center gap-1 rounded-md border border-[hsl(var(--border))] px-2 py-0.5 text-xs transition-colors disabled:cursor-default disabled:opacity-30 text-[hsl(var(--muted-foreground))] hover:enabled:border-[hsl(var(--ring))] hover:enabled:text-[hsl(var(--foreground))]"
            >
              详情
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="h-3 w-3">
                <polyline points="9 18 15 12 9 6" />
              </svg>
            </button>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* ── Uptime bars ── */}
        <UptimeBars bars={node.uptime_bars} />

      </CardContent>
    </Card>
    </>
  );
}

/* ── Page Header ──────────────────────────────────────────────────── */

function PageHeader({
  updatedAt,
  nodeCount,
}: {
  updatedAt: string | undefined;
  nodeCount: number;
}) {
  const [theme, setTheme] = useState<Theme>(getTheme);

  return (
    <header className="sticky top-0 z-30 border-b border-[hsl(var(--border))] bg-[hsl(var(--background))]/80 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-5xl items-center justify-between px-4 sm:px-6">
        {/* Left — logo + title */}
        <div className="flex items-center gap-2.5">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 32 32"
            className="h-7 w-7 shrink-0"
          >
            <rect width="32" height="32" rx="7" fill="#18181b" />
            <polyline
              points="4,16 9,16 12,9 16,23 20,12 23,16 28,16"
              fill="none"
              stroke="#fafafa"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="text-lg font-bold tracking-tight text-[hsl(var(--foreground))]">
            Pulse
          </span>
          <span className="hidden text-sm text-zinc-400 sm:inline">
            · 节点状态
          </span>
        </div>

        {/* Right — meta + actions */}
        <div className="flex items-center gap-3 text-xs text-zinc-400">
          {updatedAt && (
            <span className="hidden sm:inline">
              更新于 {formatTime(updatedAt)}
            </span>
          )}
          <span className="flex items-center gap-1.5">
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
            </span>
            {nodeCount} 节点
          </span>
          <button
            onClick={() => setTheme(toggleTheme())}
            className="rounded-md p-1.5 hover:bg-[hsl(var(--accent))] transition-colors"
            title={theme === "dark" ? "切换浅色模式" : "切换深色模式"}
          >
            {theme === "dark" ? (
              <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364-6.364l-.707.707M6.343 17.657l-.707.707M17.657 17.657l-.707-.707M6.343 6.343l-.707-.707M12 8a4 4 0 100 8 4 4 0 000-8z" />
              </svg>
            ) : (
              <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
              </svg>
            )}
          </button>
        </div>
      </div>
    </header>
  );
}

/* ── Empty State ──────────────────────────────────────────────────── */

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <svg
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        className="mb-4 h-12 w-12 text-zinc-600"
      >
        <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
        <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
        <line x1="6" y1="6" x2="6.01" y2="6" />
        <line x1="6" y1="18" x2="6.01" y2="18" />
      </svg>
      <p className="text-sm text-zinc-400">暂无节点检测数据</p>
    </div>
  );
}

/* ── Page Footer ──────────────────────────────────────────────────── */

function PageFooter() {
  return (
    <footer className="border-t border-[hsl(var(--border))] py-6 text-center text-xs text-zinc-500">
      Pulse · 节点状态
    </footer>
  );
}

/* ── Main Page Component ──────────────────────────────────────────── */

export default function StatPage() {
  const [data, setData] = useState<StatData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [metricsMap, setMetricsMap] = useState<Map<string, NodeMetrics>>(new Map());
  const [prevMetricsMap, setPrevMetricsMap] = useState<Map<string, NodeMetrics>>(new Map());

  useEffect(() => {
    let es: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      es = new EventSource("/v1/stat/stream");

      es.addEventListener("init", (e) => {
        try {
          const json: StatData = JSON.parse(e.data);
          setData(json);
          setError(null);
        } catch {}
        setLoading(false);
      });

      es.addEventListener("metrics", (e) => {
        try {
          const items: NodeMetrics[] = JSON.parse(e.data);
          setMetricsMap((prev) => {
            setPrevMetricsMap(new Map(prev));
            const next = new Map(prev);
            for (const item of items) next.set(item.node_id, item);
            return next;
          });
        } catch {}
      });

      es.onerror = () => {
        es?.close();
        setError("连接断开，正在重连…");
        retryTimer = setTimeout(connect, 5_000);
      };
    };

    connect();
    return () => {
      es?.close();
      if (retryTimer) clearTimeout(retryTimer);
    };
  }, []);

  /* ── Derived values ── */
  const avgUptimeDisplay =
    data && data.avg_uptime_pct >= 0 ? `${data.avg_uptime_pct}` : "—";
  const avgUptimeColor =
    data && data.avg_uptime_pct >= 0
      ? uptimeTextColor(data.avg_uptime_pct)
      : "text-zinc-500";

  return (
    <div className="flex h-screen flex-col overflow-y-auto bg-[hsl(var(--background))] text-[hsl(var(--foreground))]">
      <PageHeader
        updatedAt={data?.updated_at}
        nodeCount={data?.node_count ?? 0}
      />

      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-6 sm:px-6">
        {loading ? (
          <LoadingSkeleton />
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-24 text-center">
            <p className="text-sm text-red-400">加载失败: {error}</p>
          </div>
        ) : !data || !data.nodes.length ? (
          <EmptyState />
        ) : (
          <div className="space-y-6">
            {/* ── Summary Stats Row ── */}
            <div className="grid grid-cols-2 gap-3 sm:gap-4">
              <SummaryCard
                label="节点总数"
                value={data.node_count}
              />
              <SummaryCard
                label="平均可用率"
                value={avgUptimeDisplay}
                suffix={data.avg_uptime_pct >= 0 ? "%" : undefined}
                colorClass={avgUptimeColor}
              />
            </div>

            {/* ── Node Cards ── */}
            <div className="grid grid-cols-2 gap-3">
              {data.nodes.map((node) => (
                <NodeCard key={node.id} node={node} metrics={metricsMap.get(node.id)} prevMetrics={prevMetricsMap.get(node.id)} />
              ))}
            </div>
          </div>
        )}
      </main>

      <PageFooter />
    </div>
  );
}
