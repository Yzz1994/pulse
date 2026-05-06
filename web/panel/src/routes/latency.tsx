import { useEffect, useState, useCallback } from "react";
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
import { Button } from "@/components/ui";
import { api, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";

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

const ISP_LABEL: Record<string, string> = { ct: "上海电信", cu: "上海联通", cm: "上海移动" };
const ISP_COLOR_BASE: Record<string, string> = { ct: "#ef4444", cu: "#3b82f6", cm: "#22c55e" };

// 每个 ISP 固定色系，同 ISP 多节点用实线/虚线区分，颜色只在亮度上微调
const ISP_PALETTE: Record<string, string[]> = {
  ct: ["#ef4444", "#f87171", "#fca5a5"], // 电信 红
  cu: ["#3b82f6", "#60a5fa", "#93c5fd"], // 联通 蓝
  cm: ["#22c55e", "#4ade80", "#86efac"], // 移动 绿
};

function lineColor(nodeIdx: number, isp: string): string {
  const palette = ISP_PALETTE[isp] ?? ISP_PALETTE.ct!;
  return palette[nodeIdx % palette.length]!;
}

// ── 时间范围选项 ─────────────────────────────────────────────────

const RANGES = [
  { label: "1h",  minutes: 60 },
  { label: "3h",  minutes: 180 },
  { label: "6h",  minutes: 360 },
  { label: "24h", minutes: 1440 },
];

// ── 数据转换 ─────────────────────────────────────────────────────

type ChartPoint = Record<string, number | string | null>;

function buildChartData(samples: LatencySample[]): { data: ChartPoint[]; keys: string[] } {
  if (!samples.length) return { data: [], keys: [] };

  // 所有唯一的 node+isp 组合
  const keySet = new Set<string>();
  samples.forEach((s) => keySet.add(`${s.node_name}-${ISP_LABEL[s.isp] ?? s.isp}`));
  const keys = [...keySet].sort();

  // 按时间桶聚合（同一分钟取平均）
  const buckets = new Map<string, Map<string, number[]>>();
  samples.forEach((s) => {
    const t = s.sampled_at.slice(0, 16); // "2006-01-02T15:04"
    const key = `${s.node_name}-${ISP_LABEL[s.isp] ?? s.isp}`;
    if (!buckets.has(t)) buckets.set(t, new Map());
    const m = buckets.get(t)!;
    if (s.rtt_ms !== null) {
      if (!m.has(key)) m.set(key, []);
      m.get(key)!.push(s.rtt_ms);
    }
  });

  const sortedTimes = [...buckets.keys()].sort();
  const data: ChartPoint[] = sortedTimes.map((t) => {
    const point: ChartPoint = { t: t.slice(11) }; // "HH:MM"
    const m = buckets.get(t)!;
    keys.forEach((k) => {
      const vals = m.get(k);
      point[k] = vals?.length
        ? Math.round(vals.reduce((a, b) => a + b, 0) / vals.length)
        : null;
    });
    return point;
  });

  return { data, keys };
}

// ── 自定义 Tooltip ───────────────────────────────────────────────

function CustomTooltip({ active, payload, label }: {
  active?: boolean;
  payload?: Array<{ name: string; value: number | null; color: string }>;
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  const valid = payload.filter((p) => p.value !== null && p.value !== undefined);
  if (!valid.length) return null;
  return (
    <div className="rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--background))] p-3 shadow-md text-xs">
      <p className="mb-2 font-medium text-[hsl(var(--muted-foreground))]">{label}</p>
      {valid.map((p) => (
        <div key={p.name} className="flex items-center justify-between gap-6">
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-full" style={{ background: p.color }} />
            <span className="text-[hsl(var(--foreground))]">{p.name}</span>
          </span>
          <span className="font-bold tabular-nums" style={{ color: p.color }}>
            {p.value} ms
          </span>
        </div>
      ))}
    </div>
  );
}

// ── 主页面 ────────────────────────────────────────────────────────

export default function LatencyPage() {
  const navigate = useNavigate();
  const [rangeMinutes, setRangeMinutes] = useState(60);
  const [loading, setLoading] = useState(false);
  const [samples, setSamples] = useState<LatencySample[]>([]);
  const [error, setError] = useState<string | null>(null);

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

  const { data, keys } = buildChartData(samples);

  // 按节点名排序，取唯一节点列表用于着色
  const nodeNames = [...new Set(samples.map((s) => s.node_name))].sort();

  return (
    <div className="space-y-4 p-6">
      {/* 标题栏 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">延迟监控</h1>
          <p className="text-sm text-[hsl(var(--muted-foreground))]">
            各节点 → 上海三网（电信 / 联通 / 移动）TCP 延迟
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

      {/* 错误提示 */}
      {error && (
        <div className="rounded-lg border border-[hsl(var(--destructive))] bg-[hsl(var(--destructive))]/10 px-4 py-3 text-sm text-[hsl(var(--destructive))]">
          {error}
        </div>
      )}

      {/* 图表 */}
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
              <Tooltip content={<CustomTooltip />} />
              <Legend
                formatter={(value) => (
                  <span className="text-xs text-[hsl(var(--foreground))]">{value}</span>
                )}
              />
              {keys.map((key) => {
                // key 格式: "节点名-ISP中文"
                const dashIdx = key.lastIndexOf("-");
                const nodeName = key.slice(0, dashIdx);
                const ispCn = key.slice(dashIdx + 1);
                const isp = Object.entries(ISP_LABEL).find(([, v]) => v === ispCn)?.[0] ?? "ct";
                const nodeIdx = nodeNames.indexOf(nodeName);
                return (
                  <Line
                    key={key}
                    type="monotone"
                    dataKey={key}
                    name={key}
                    stroke={lineColor(nodeIdx, isp)}
                    strokeWidth={nodeIdx === 0 ? 2 : 1.5}
                    strokeDasharray={nodeIdx === 0 ? undefined : nodeIdx === 1 ? "5 3" : "2 2"}
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

      {/* 图例补充说明 */}
      {data.length > 0 && (
        <p className="text-xs text-[hsl(var(--muted-foreground))]">
          每分钟采样一次 · 数据保留 7 天 · null 值表示节点超时或不可达
        </p>
      )}
    </div>
  );
}
