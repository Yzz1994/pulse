import { useEffect, useState, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  Badge,
} from "@/components/ui";
import { api, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";

// ── 类型定义 ─────────────────────────────────────────────────────

interface NodeItem {
  id: string;
  name: string;
}

interface NodesResponse {
  nodes: NodeItem[];
}

interface TracerouteHop {
  hop: number;
  ip?: string;
  rtt_ms?: number[];
  timeout?: boolean;
  geo?: {
    country_code?: string;
    country_name?: string;
    city?: string;
    asn?: number;
    asn_org?: string;
  };
  network?: string;
}

interface TracerouteResult {
  id: string;
  node_id: string;
  direction: "inbound" | "outbound";
  target: string;
  hops: string; // JSON 字符串
  quality: string;
  created_at: string;
}

interface TracerouteResultsResponse {
  results: TracerouteResult[];
}

// ── 工具函数 ─────────────────────────────────────────────────────

/** 国旗 emoji（通过区域指示符字母拼合） */
function countryFlag(code: string): string {
  if (!code || code.length !== 2) return "";
  const offset = 0x1f1e6 - 65;
  return String.fromCodePoint(code.toUpperCase().charCodeAt(0) + offset) +
    String.fromCodePoint(code.toUpperCase().charCodeAt(1) + offset);
}

/** 判断是否为私有/保留 IP */
function isPrivateIP(ip: string): boolean {
  if (ip.startsWith("10.") || ip.startsWith("127.") || ip === "::1") return true;
  if (ip.startsWith("192.168.")) return true;
  if (ip.startsWith("11.")) return true;
  if (ip.startsWith("100.")) {
    const second = parseInt(ip.split(".")[1] ?? "0");
    if (second >= 64 && second <= 127) return true;
  }
  const parts = ip.split(".").map(Number);
  if (
    parts.length === 4 &&
    (parts[0] ?? 0) === 172 &&
    (parts[1] ?? 0) >= 16 &&
    (parts[1] ?? 0) <= 31
  ) return true;
  return false;
}

/** 格式化时间 */
function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

/** 平均 RTT */
function avgRtt(rtts?: number[]): string | null {
  if (!rtts?.length) return null;
  return (rtts.reduce((a, b) => a + b, 0) / rtts.length).toFixed(1);
}

/** 线路质量徽章颜色 */
function qualityColor(quality: string): string {
  if (quality === "CN2 GIA") return "text-emerald-600 bg-emerald-500/10";
  if (quality === "CN2 GT") return "text-blue-600 bg-blue-500/10";
  return "text-amber-600 bg-amber-500/10";
}

/** 网络类型徽章颜色 */
function networkBadgeColor(type: string): string {
  switch (type) {
    case "CN2":  return "text-emerald-600 bg-emerald-500/10";
    case "163":  return "text-amber-600 bg-amber-500/10";
    case "CU2":  return "text-emerald-600 bg-emerald-500/10";
    case "CU":   return "text-blue-600 bg-blue-500/10";
    case "CMI":  return "text-purple-600 bg-purple-500/10";
    default:     return "text-[hsl(var(--muted-foreground))] bg-[hsl(var(--muted))]";
  }
}

// ── 跳数详情表格 ─────────────────────────────────────────────────

function HopsTable({ hopsJson }: { hopsJson: string }) {
  let hops: TracerouteHop[] = [];
  try {
    hops = JSON.parse(hopsJson) as TracerouteHop[];
  } catch {
    return (
      <p className="py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">
        跳数数据解析失败
      </p>
    );
  }

  if (hops.length === 0) {
    return (
      <p className="py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">
        暂无跳数数据
      </p>
    );
  }

  return (
    <table className="w-full text-xs font-mono">
      <thead>
        <tr className="border-b border-[hsl(var(--border))] bg-[hsl(var(--muted))]">
          <th className="py-2 pl-3 pr-2 text-left font-medium text-[hsl(var(--muted-foreground))]">#</th>
          <th className="py-2 pr-2 text-left font-medium text-[hsl(var(--muted-foreground))]">IP</th>
          <th className="py-2 pr-2 text-left font-medium text-[hsl(var(--muted-foreground))]">网络</th>
          <th className="py-2 pr-2 text-left font-medium text-[hsl(var(--muted-foreground))]">地区 / ASN</th>
          <th className="py-2 pr-3 text-right font-medium text-[hsl(var(--muted-foreground))]">平均 RTT</th>
        </tr>
      </thead>
      <tbody>
        {hops.map((hop) => {
          const geo = hop.geo;
          const netType = hop.network ?? (
            hop.ip && !isPrivateIP(hop.ip)
              ? (hop.ip.startsWith("59.43.") ? "CN2"
                : hop.ip.startsWith("202.97.") ? "163"
                : hop.ip.startsWith("219.158.") ? "CU"
                : "")
              : ""
          );
          return (
            <tr
              key={hop.hop}
              className="border-b border-[hsl(var(--border))] last:border-0 hover:bg-[hsl(var(--muted)/0.4)]"
            >
              <td className="py-2 pl-3 pr-2 text-[hsl(var(--muted-foreground))]">{hop.hop}</td>
              <td className="py-2 pr-2">
                {hop.timeout ? (
                  <span className="text-[hsl(var(--muted-foreground))]">* * *</span>
                ) : (
                  <span>{hop.ip}</span>
                )}
              </td>
              <td className="py-2 pr-2">
                {hop.ip && isPrivateIP(hop.ip) ? (
                  <span className="text-[hsl(var(--muted-foreground))] opacity-40">内网</span>
                ) : netType ? (
                  <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${networkBadgeColor(netType)}`}>
                    {netType}
                  </span>
                ) : geo?.asn_org ? (
                  <span className="text-[hsl(var(--muted-foreground))] opacity-60" title={geo.asn_org}>
                    {geo.asn_org.length > 14 ? geo.asn_org.slice(0, 13) + "…" : geo.asn_org}
                  </span>
                ) : null}
              </td>
              <td className="py-2 pr-2">
                {hop.ip && isPrivateIP(hop.ip) ? null : geo ? (
                  <span className="text-[hsl(var(--muted-foreground))]">
                    {geo.country_code && (
                      <span className="mr-1">{countryFlag(geo.country_code)}</span>
                    )}
                    {[geo.country_name, geo.city].filter(Boolean).join(" · ")}
                    {geo.asn_org && (
                      <span className="ml-1.5 opacity-70">{geo.asn_org}</span>
                    )}
                  </span>
                ) : null}
              </td>
              <td className="py-2 pr-3 text-right">
                {hop.timeout || !hop.rtt_ms?.length ? (
                  <span className="text-[hsl(var(--muted-foreground))]">—</span>
                ) : (
                  <span
                    className="text-[hsl(var(--foreground))]"
                    title={hop.rtt_ms.map((v) => v.toFixed(2) + " ms").join("  ")}
                  >
                    {avgRtt(hop.rtt_ms)} ms
                  </span>
                )}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// ── 单条记录行 ───────────────────────────────────────────────────

function ResultRow({
  result,
  nodeName,
  onDelete,
}: {
  result: TracerouteResult;
  nodeName: string;
  onDelete: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!confirm("确认删除这条追踪记录？")) return;
    setDeleting(true);
    try {
      await api.del(`/nodes/${result.node_id}/traceroute/results/${result.id}`);
      onDelete(result.id);
    } catch {
      // 静默失败
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="border-b border-[hsl(var(--border))] last:border-0">
      {/* 摘要行 */}
      <div
        className="flex cursor-pointer items-center gap-3 px-4 py-3 hover:bg-[hsl(var(--muted)/0.4)] transition-colors"
        onClick={() => setExpanded((v) => !v)}
      >
        {/* 节点名称 */}
        <span className="w-28 shrink-0 text-sm font-medium truncate" title={nodeName}>
          {nodeName}
        </span>

        {/* 方向标签 */}
        <span
          className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
            result.direction === "inbound"
              ? "text-purple-600 bg-purple-500/10"
              : "text-sky-600 bg-sky-500/10"
          }`}
        >
          {result.direction === "inbound" ? "回程" : "去程"}
        </span>

        {/* 目标地址 */}
        <span className="flex-1 text-sm text-[hsl(var(--muted-foreground))] truncate" title={result.target}>
          {result.target}
        </span>

        {/* 线路质量 */}
        {result.quality ? (
          <span
            className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold ${qualityColor(result.quality)}`}
          >
            {result.quality}
          </span>
        ) : null}

        {/* 时间 */}
        <span className="shrink-0 text-xs text-[hsl(var(--muted-foreground))]">
          {formatTime(result.created_at)}
        </span>

        {/* 删除按钮 */}
        <button
          onClick={handleDelete}
          disabled={deleting}
          className="shrink-0 rounded p-1 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--destructive))] hover:bg-[hsl(var(--destructive)/0.1)] transition-colors disabled:opacity-40"
          title="删除"
        >
          <TrashIcon className="h-3.5 w-3.5" />
        </button>

        {/* 展开按钮 */}
        <span
          className={`shrink-0 text-[hsl(var(--muted-foreground))] transition-transform duration-200 ${expanded ? "rotate-180" : ""}`}
        >
          <ChevronDownIcon className="h-4 w-4" />
        </span>
      </div>

      {/* 跳数详情 */}
      {expanded && (
        <div className="border-t border-[hsl(var(--border))] bg-[hsl(var(--muted)/0.3)]">
          <HopsTable hopsJson={result.hops} />
        </div>
      )}
    </div>
  );
}

// ── 主页面 ───────────────────────────────────────────────────────

export default function TraceRoutePage() {
  const navigate = useNavigate();

  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [nodesLoading, setNodesLoading] = useState(true);
  const [selectedNodeId, setSelectedNodeId] = useState<string>("all");

  const [results, setResults] = useState<TracerouteResult[]>([]);
  const [resultsLoading, setResultsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  /** 处理鉴权错误统一跳转 */
  function handleAuthError(err: unknown) {
    if (err instanceof AuthError) {
      clearToken();
      navigate({ to: "/panel/login" });
    }
  }

  /** 拉取节点列表 */
  useEffect(() => {
    setNodesLoading(true);
    api
      .get<NodesResponse>("/nodes")
      .then((res) => setNodes(res.nodes ?? []))
      .catch((err) => handleAuthError(err))
      .finally(() => setNodesLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** 拉取路由追踪历史记录 */
  const fetchResults = useCallback(async () => {
    setResultsLoading(true);
    setError(null);
    try {
      if (selectedNodeId === "all") {
        // 使用汇总端点一次获取所有节点的最新快照
        const res = await api.get<{ snapshots: Record<string, TracerouteResult[]> }>(
          "/nodes/traceroute/latest"
        );
        const merged: TracerouteResult[] = Object.values(res.snapshots ?? {}).flat();
        merged.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
        setResults(merged);
      } else {
        const res = await api.get<TracerouteResultsResponse>(
          `/nodes/${selectedNodeId}/traceroute/results?limit=50`
        );
        setResults(res.results ?? []);
      }
    } catch (err: unknown) {
      handleAuthError(err);
      setError(err instanceof Error ? err.message : "请求失败");
    } finally {
      setResultsLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedNodeId]);

  useEffect(() => {
    fetchResults();
  }, [fetchResults]);

  /** 节点名称查找 */
  function getNodeName(nodeId: string): string {
    return nodes.find((n) => n.id === nodeId)?.name ?? nodeId;
  }

  return (
    <div className="p-6 space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">路由追踪</h1>
        <p className="text-sm text-[hsl(var(--muted-foreground))] mt-1">历史追踪记录</p>
      </div>

      {/* 筛选栏 */}
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium text-[hsl(var(--foreground))]">
          节点
        </label>
        <select
          value={selectedNodeId}
          onChange={(e) => setSelectedNodeId(e.target.value)}
          disabled={nodesLoading}
          className="rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3 py-1.5 text-sm text-[hsl(var(--foreground))] focus:outline-none focus:ring-2 focus:ring-[hsl(var(--ring))] disabled:opacity-50"
        >
          <option value="all">全部节点</option>
          {nodes.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </select>

        <button
          onClick={fetchResults}
          disabled={resultsLoading || nodesLoading}
          className="rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3 py-1.5 text-sm text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))] disabled:opacity-50 transition-colors"
        >
          {resultsLoading ? "加载中…" : "刷新"}
        </button>
      </div>

      {/* 记录列表 */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">追踪记录</CardTitle>
          <CardDescription>
            {resultsLoading
              ? "正在加载…"
              : `共 ${results.length} 条记录`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {/* 错误提示 */}
          {error && (
            <div className="px-4 py-3 text-sm text-[hsl(var(--destructive))]">
              {error}
            </div>
          )}

          {/* 加载骨架 */}
          {resultsLoading && (
            <div className="space-y-0">
              {[1, 2, 3].map((i) => (
                <div
                  key={i}
                  className="flex items-center gap-3 border-b border-[hsl(var(--border))] px-4 py-3 last:border-0"
                >
                  <div className="h-4 w-24 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-4 w-12 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-4 flex-1 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-4 w-16 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-4 w-28 animate-pulse rounded bg-[hsl(var(--muted))]" />
                </div>
              ))}
            </div>
          )}

          {/* 空状态 */}
          {!resultsLoading && !error && results.length === 0 && (
            <div className="py-12 text-center text-sm text-[hsl(var(--muted-foreground))]">
              暂无追踪记录
            </div>
          )}

          {/* 数据列表 */}
          {!resultsLoading && results.length > 0 && (
            <div>
              {/* 表头 */}
              <div className="flex items-center gap-3 border-b border-[hsl(var(--border))] bg-[hsl(var(--muted)/0.5)] px-4 py-2 text-xs font-medium text-[hsl(var(--muted-foreground))]">
                <span className="w-28 shrink-0">节点</span>
                <span className="w-12 shrink-0">方向</span>
                <span className="flex-1">目标</span>
                <span className="shrink-0">线路</span>
                <span className="shrink-0 w-32 text-right">时间</span>
                <span className="shrink-0 w-4" />
              </div>

              {results.map((r) => (
                <ResultRow
                  key={r.id}
                  result={r}
                  nodeName={getNodeName(r.node_id)}
                  onDelete={(id) => setResults((prev) => prev.filter((x) => x.id !== id))}
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

    </div>
  );
}

// ── 图标 ─────────────────────────────────────────────────────────

function ChevronDownIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <polyline points="6 9 12 15 18 9" />
    </svg>
  );
}

function TrashIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" {...props}>
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
      <path d="M10 11v6M14 11v6" />
      <path d="M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2" />
    </svg>
  );
}
