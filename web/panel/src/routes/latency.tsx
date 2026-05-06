import { useEffect, useState, useCallback, useMemo } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { Button, Badge } from "@/components/ui";
import { api, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";
import type { Node, NodesResponse } from "@/lib/types";

// ── 类型 ────────────────────────────────────────────────────────

interface LatencySample {
  node_id: string;
  node_name: string;
  isp: "ct" | "cu" | "cm";
  rtt_ms: number | null;
  sampled_at: string;
}

interface LatencyResponse {
  samples: LatencySample[];
}

// ── 常量 ────────────────────────────────────────────────────────

const ISP_LABEL: Record<string, string> = { ct: "电信", cu: "联通", cm: "移动" };

const ISP_COLOR: Record<string, string> = {
  ct: "#ef4444",
  cu: "#3b82f6",
  cm: "#22c55e",
};

const ISP_DASH: Record<string, string | undefined> = {
  ct: undefined,
  cu: "6 3",
  cm: "2 3",
};

const RANGES = [
  { label: "1h",  minutes: 60 },
  { label: "3h",  minutes: 180 },
  { label: "6h",  minutes: 360 },
  { label: "24h", minutes: 1440 },
];

// ── 构建图表数据 ─────────────────────────────────────────────────

type ChartPoint = Record<string, number | string | null>;

function buildChartData(samples: LatencySample[]): {
  data: ChartPoint[];
  /** "nodeId:isp" 格式的所有 series key */
  seriesKeys: string[];
  nodeMap: Map<string, string>; // nodeId → nodeName
} {
  if (!samples.length) return { data: [], seriesKeys: [], nodeMap: new Map() };

  const nodeMap = new Map<string, string>();
  const keySet = new Set<string>();
  samples.forEach((s) => {
    nodeMap.set(s.node_id, s.node_name);
    keySet.add(`${s.node_id}:${s.isp}`);
  });
  const seriesKeys = [...keySet].sort();

  const buckets = new Map<string, Map<string, number[]>>();
  samples.forEach((s) => {
    const t = s.sampled_at.slice(0, 16);
    const key = `${s.node_id}:${s.isp}`;
    if (!buckets.has(t)) buckets.set(t, new Map());
    const m = buckets.get(t)!;
    if (s.rtt_ms !== null) {
      if (!m.has(key)) m.set(key, []);
      m.get(key)!.push(s.rtt_ms);
    }
  });

  const sortedTimes = [...buckets.keys()].sort();
  const data: ChartPoint[] = sortedTimes.map((t) => {
    const point: ChartPoint = { t: t.slice(11) };
    const m = buckets.get(t)!;
    seriesKeys.forEach((k) => {
      const vals = m.get(k);
      point[k] = vals?.length
        ? Math.round(vals.reduce((a, b) => a + b, 0) / vals.length)
        : null;
    });
    return point;
  });

  return { data, seriesKeys, nodeMap };
}

// ── Tooltip ──────────────────────────────────────────────────────

function CustomTooltip({ active, payload, label, nodeMap }: {
  active?: boolean;
  payload?: Array<{ name: string; value: number | null; color: string }>;
  label?: string;
  nodeMap: Map<string, string>;
}) {
  if (!active || !payload?.length) return null;
  const valid = payload.filter((p) => p.value !== null && p.value !== undefined);
  if (!valid.length) return null;
  return (
    <div className="rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--background))] p-3 shadow-md text-xs min-w-[160px]">
      <p className="mb-2 font-medium text-[hsl(var(--muted-foreground))]">{label}</p>
      {valid.map((p) => {
        const [nodeId, isp] = p.name.split(":");
        const nodeName = nodeMap.get(nodeId ?? "") ?? nodeId;
        return (
          <div key={p.name} className="flex items-center justify-between gap-4 py-0.5">
            <span className="flex items-center gap-1.5 min-w-0">
              <span className="inline-block h-2 w-2 shrink-0 rounded-full" style={{ background: p.color }} />
              <span className="truncate text-[hsl(var(--foreground))]">
                {nodeName} · {ISP_LABEL[isp ?? ""] ?? isp}
              </span>
            </span>
            <span className="font-bold tabular-nums shrink-0" style={{ color: p.color }}>
              {p.value} ms
            </span>
          </div>
        );
      })}
    </div>
  );
}

// ── 节点筛选 Pill ─────────────────────────────────────────────────

function NodeFilter({
  nodes,
  selected,
  onToggle,
}: {
  nodes: Node[];
  selected: Set<string>;
  onToggle: (id: string) => void;
}) {
  if (!nodes.length) return null;
  return (
    <div className="flex flex-wrap gap-1.5">
      {nodes.map((n) => {
        const active = selected.has(n.id);
        return (
          <button
            key={n.id}
            onClick={() => onToggle(n.id)}
            className={[
              "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
              active
                ? "border-[hsl(var(--primary))] bg-[hsl(var(--primary))] text-[hsl(var(--primary-foreground))]"
                : "border-[hsl(var(--border))] bg-transparent text-[hsl(var(--muted-foreground))] hover:border-[hsl(var(--foreground)/0.4)] hover:text-[hsl(var(--foreground))]",
            ].join(" ")}
          >
            {n.name}
          </button>
        );
      })}
    </div>
  );
}

// ── 图例 formatter ───────────────────────────────────────────────

function makeLegendFormatter(nodeMap: Map<string, string>) {
  return (value: string) => {
    const [nodeId, isp] = value.split(":");
    const nodeName = nodeMap.get(nodeId ?? "") ?? nodeId;
    return (
      <span className="text-xs text-[hsl(var(--foreground))]">
        {nodeName} · {ISP_LABEL[isp ?? ""] ?? isp}
      </span>
    );
  };
}

// ── 主页面 ────────────────────────────────────────────────────────

export default function LatencyPage() {
  const navigate = useNavigate();
  const [rangeMinutes, setRangeMinutes] = useState(60);
  const [loading, setLoading] = useState(false);
  const [samples, setSamples] = useState<LatencySample[]>([]);
  const [error, setError] = useState<string | null>(null);

  // 非落地节点列表
  const [nonLandingNodes, setNonLandingNodes] = useState<Node[]>([]);
  // 当前勾选的节点 ID 集合（空 = 全选）
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  // 拉取节点列表，过滤落地机
  useEffect(() => {
    api.get<NodesResponse>("/nodes")
      .then((res) => {
        const nodes = (res.nodes ?? []).filter((n) => !n.is_landing);
        setNonLandingNodes(nodes);
        setSelectedIds(new Set(nodes.map((n) => n.id)));
      })
      .catch((err) => {
        if (err instanceof AuthError) { clearToken(); navigate({ to: "/panel/login" }); }
      });
  }, [navigate]);

  const fetchData = useCallback(async (minutes: number) => {
    setLoading(true);
    setError(null);
    const to = new Date();
    const from = new Date(to.getTime() - minutes * 60_000);
    try {
      const res = await api.get<LatencyResponse>(
        `/nodes/latency?from=${from.toISOString()}&to=${to.toISOString()}`
      );
      setSamples(res.samples ?? []);
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [navigate]);

  useEffect(() => {
    fetchData(rangeMinutes);
    const timer = setInterval(() => fetchData(rangeMinutes), 60_000);
    return () => clearInterval(timer);
  }, [rangeMinutes, fetchData]);

  const toggleNode = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        // 至少保留一个
        if (next.size > 1) next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  // 过滤：只保留非落地机 + 已选中节点的 samples
  const nonLandingIds = useMemo(
    () => new Set(nonLandingNodes.map((n) => n.id)),
    [nonLandingNodes]
  );

  const filteredSamples = useMemo(
    () => samples.filter((s) => nonLandingIds.has(s.node_id) && selectedIds.has(s.node_id)),
    [samples, nonLandingIds, selectedIds]
  );

  const { data, seriesKeys, nodeMap } = useMemo(
    () => buildChartData(filteredSamples),
    [filteredSamples]
  );

  const legendFormatter = useMemo(() => makeLegendFormatter(nodeMap), [nodeMap]);

  return (
    <div className="space-y-4 p-6">
      {/* ── 页头 ─────────────────────────────────────────────── */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">延迟监控</h1>
          <p className="text-sm text-[hsl(var(--muted-foreground))]">
            各节点 → 上海三网（电信 / 联通 / 移动）TCP 延迟，仅统计非落地节点
          </p>
        </div>
        <div className="flex items-center gap-2">
          {RANGES.map((r) => (
            <Button
              key={r.label}
              variant={rangeMinutes === r.minutes ? "default" : "outline"}
              size="sm"
              onClick={() => setRangeMinutes(r.minutes)}
            >
              {r.label}
            </Button>
          ))}
          <Button
            variant="outline"
            size="sm"
            onClick={() => fetchData(rangeMinutes)}
            disabled={loading}
          >
            {loading ? "加载中…" : "刷新"}
          </Button>
        </div>
      </div>

      {/* ── 节点筛选 ──────────────────────────────────────────── */}
      {nonLandingNodes.length > 1 && (
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-xs text-[hsl(var(--muted-foreground))] shrink-0">节点筛选</span>
          <NodeFilter
            nodes={nonLandingNodes}
            selected={selectedIds}
            onToggle={toggleNode}
          />
          {selectedIds.size < nonLandingNodes.length && (
            <button
              className="text-xs text-[hsl(var(--muted-foreground))] underline underline-offset-2 hover:text-[hsl(var(--foreground))]"
              onClick={() => setSelectedIds(new Set(nonLandingNodes.map((n) => n.id)))}
            >
              全选
            </button>
          )}
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-[hsl(var(--destructive))] bg-[hsl(var(--destructive))]/10 px-4 py-3 text-sm text-[hsl(var(--destructive))]">
          {error}
        </div>
      )}

      {/* ── 图表 ─────────────────────────────────────────────── */}
      <div className="rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--background))] p-4">
        {data.length === 0 && !loading ? (
          <div className="flex h-64 items-center justify-center text-sm text-[hsl(var(--muted-foreground))]">
            暂无数据，节点将在下次采样后显示（每分钟一次）
          </div>
        ) : (
          <ResponsiveContainer width="100%" height={360}>
            <LineChart data={data} margin={{ top: 4, right: 16, left: 0, bottom: 0 }}>
              <CartesianGrid
                strokeDasharray="3 3"
                stroke="hsl(var(--border))"
                opacity={0.5}
              />
              <XAxis
                dataKey="t"
                tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
                tickLine={false}
                axisLine={false}
                interval="preserveStartEnd"
              />
              <YAxis
                tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
                tickLine={false}
                axisLine={false}
                unit=" ms"
                width={55}
              />
              <Tooltip content={<CustomTooltip nodeMap={nodeMap} />} />
              <Legend formatter={legendFormatter} />
              {seriesKeys.map((key) => {
                const [, isp] = key.split(":");
                return (
                  <Line
                    key={key}
                    type="monotone"
                    dataKey={key}
                    name={key}
                    stroke={ISP_COLOR[isp ?? "ct"] ?? ISP_COLOR.ct}
                    strokeWidth={1.5}
                    strokeDasharray={ISP_DASH[isp ?? "ct"]}
                    dot={false}
                    activeDot={{ r: 4 }}
                    connectNulls={false}
                  />
                );
              })}
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>

      {data.length > 0 && (
        <p className="text-xs text-[hsl(var(--muted-foreground))]">
          每分钟采样一次 · 数据保留 7 天 · null 值表示节点超时或不可达
        </p>
      )}
    </div>
  );
}
