import { useEffect, useState, useCallback, useRef } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  toast,
} from "@/components/ui";
import { api, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";

// ── 类型定义 ─────────────────────────────────────────────────────

interface NodeItem {
  id: string;
  name: string;
  base_url: string;
}

interface NodesResponse {
  nodes: NodeItem[];
}

interface Schedule {
  interval_hours: number;
  last_run_at: string | null;
  next_run_at: string | null;
}

interface NodeIPSentinelConfig {
  region_code: string;
  region_name: string;
}

interface IPDetectResult {
  ip: string;
  country: string;
  country_code: string;
  city: string;
  isp: string;
  org: string;
  lat: number;
  lon: number;
  timezone: string;
  detected_at: string;
}

interface IPSentinelRun {
  id: string;
  task_type: string;
  triggered_by: string;
  status: "pending" | "running" | "success" | "failed";
  output: string[];
  started_at: string;
  finished_at: string | null;
  result?: unknown;
}

interface RunsResponse {
  runs: IPSentinelRun[];
}

// ── 预设地区映射 ──────────────────────────────────────────────────

const REGION_PRESETS: { code: string; name: string }[] = [
  { code: "US", name: "United States" },
  { code: "JP", name: "Japan" },
  { code: "SG", name: "Singapore" },
  { code: "GB", name: "United Kingdom" },
  { code: "DE", name: "Germany" },
  { code: "FR", name: "France" },
  { code: "HK", name: "Hong Kong" },
  { code: "KR", name: "South Korea" },
  { code: "AU", name: "Australia" },
  { code: "CA", name: "Canada" },
  { code: "TW", name: "Taiwan" },
  { code: "NL", name: "Netherlands" },
];

// ── 工具函数 ─────────────────────────────────────────────────────

function formatTime(iso: string | null | undefined): string {
  if (!iso) return "--";
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

function calcDuration(startedAt: string, finishedAt: string | null): string {
  if (!finishedAt) return "—";
  try {
    const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime();
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  } catch {
    return "—";
  }
}

function statusBadgeClass(status: IPSentinelRun["status"] | undefined): string {
  switch (status) {
    case "pending":
    case "running":
      return "text-blue-600 bg-blue-500/10";
    case "success":
      return "text-emerald-600 bg-emerald-500/10";
    case "failed":
      return "text-red-600 bg-red-500/10";
    default:
      return "text-[hsl(var(--muted-foreground))] bg-[hsl(var(--muted))]";
  }
}

function statusLabel(status: IPSentinelRun["status"] | undefined): string {
  switch (status) {
    case "pending":  return "等待中";
    case "running":  return "运行中";
    case "success":  return "成功";
    case "failed":   return "失败";
    default:         return "--";
  }
}

function triggeredByLabel(v: string): string {
  if (v === "auto" || v === "scheduler") return "自动";
  if (v === "manual" || v === "user")    return "手动";
  return v || "—";
}

// ── 每个节点卡片的本地状态 ────────────────────────────────────────

interface NodeCardState {
  config: NodeIPSentinelConfig;
  configLoaded: boolean;
  runs: IPSentinelRun[];
  runsLoaded: boolean;
  runsExpanded: boolean;
  detectResult: IPDetectResult | null;
  detectLoading: boolean;
  runLoading: boolean;
  configSaving: boolean;
  // 当前编辑中的地区（未保存的草稿）
  draftCode: string;
  draftName: string;
}

// ── 执行记录行（可展开输出） ─────────────────────────────────────

function RunRow({ run }: { run: IPSentinelRun }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr
        className="border-b border-[hsl(var(--border))] last:border-0 hover:bg-[hsl(var(--muted)/0.4)] cursor-pointer select-none"
        onClick={() => setExpanded((v) => !v)}
      >
        <td className="py-1.5 pl-3 pr-2 text-[11px] text-[hsl(var(--muted-foreground))] whitespace-nowrap">
          {formatTime(run.started_at)}
        </td>
        <td className="py-1.5 pr-2">
          <span className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]">
            {triggeredByLabel(run.triggered_by)}
          </span>
        </td>
        <td className="py-1.5 pr-2">
          <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium ${statusBadgeClass(run.status)}`}>
            {statusLabel(run.status)}
          </span>
        </td>
        <td className="py-1.5 pr-3 text-right text-[11px] text-[hsl(var(--muted-foreground))]">
          {calcDuration(run.started_at, run.finished_at)}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b border-[hsl(var(--border))]">
          <td colSpan={4} className="px-3 pb-2 pt-1">
            <pre className="max-h-60 overflow-y-auto rounded bg-[hsl(var(--muted))] p-2.5 text-[10px] leading-relaxed font-mono whitespace-pre-wrap text-[hsl(var(--foreground))]">
              {run.output.length > 0
                ? run.output.join("\n")
                : run.result
                  ? JSON.stringify(run.result, null, 2)
                  : "(无输出)"}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}

// ── 节点卡片组件 ──────────────────────────────────────────────────

function NodeCard({
  node,
  onAuthError,
}: {
  node: NodeItem;
  onAuthError: () => void;
}) {
  const [state, setState] = useState<NodeCardState>({
    config: { region_code: "", region_name: "" },
    configLoaded: false,
    runs: [],
    runsLoaded: false,
    runsExpanded: false,
    detectResult: null,
    detectLoading: false,
    runLoading: false,
    configSaving: false,
    draftCode: "",
    draftName: "",
  });

  // 轮询计时器 ref
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollCountRef = useRef(0);

  // 工具函数：更新 state 的部分字段
  function patch(partial: Partial<NodeCardState>) {
    setState((prev) => ({ ...prev, ...partial }));
  }

  // 错误处理
  function handleError(err: unknown, fallback: string) {
    if (err instanceof AuthError) {
      onAuthError();
      return;
    }
    toast(err instanceof Error ? err.message : fallback, "error");
  }

  // ── 加载配置 ────────────────────────────────────────────────────

  const fetchConfig = useCallback(async () => {
    try {
      const data = await api.get<NodeIPSentinelConfig>(`/nodes/${node.id}/ip-sentinel/config`);
      patch({
        config: data,
        configLoaded: true,
        draftCode: data.region_code ?? "",
        draftName: data.region_name ?? "",
      });
    } catch (err) {
      if (err instanceof Error && err.message.includes("404")) {
        patch({ configLoaded: true });
      } else {
        handleError(err, "加载配置失败");
        patch({ configLoaded: true });
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node.id]);

  // ── 加载执行记录 ─────────────────────────────────────────────────

  const fetchRuns = useCallback(async (): Promise<IPSentinelRun[]> => {
    try {
      const data = await api.get<RunsResponse>(`/nodes/${node.id}/ip-sentinel/runs`);
      const runs = data.runs ?? [];

      // 从历史记录中找最近一次成功的 detect 结果，自动填充 IP 展示
      const latestDetect = runs.find(
        (r) => r.task_type === "detect" && r.status === "success" && r.result
      );
      const detectResult = (latestDetect?.result ?? null) as IPDetectResult | null;

      patch({ runs, runsLoaded: true, ...(detectResult ? { detectResult } : {}) });
      return runs;
    } catch (err) {
      handleError(err, "加载执行记录失败");
      patch({ runsLoaded: true });
      return [];
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node.id]);

  // ── 挂载时并发加载 ───────────────────────────────────────────────

  useEffect(() => {
    fetchConfig();
    fetchRuns();

    // 清理轮询
    return () => {
      if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── 轮询逻辑（运行中时每 3 秒查一次，最多 5 分钟） ───────────────

  function startPolling() {
    pollCountRef.current = 0;
    schedulePoll();
  }

  function schedulePoll() {
    if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    pollTimerRef.current = setTimeout(async () => {
      pollCountRef.current += 1;
      const runs = await fetchRuns();
      const latest = runs[0];
      const isRunning = latest?.status === "pending" || latest?.status === "running";
      // 最多轮询 100 次（5 分钟）
      if (isRunning && pollCountRef.current < 100) {
        schedulePoll();
      }
    }, 3000);
  }

  // ── 检测 IP ──────────────────────────────────────────────────────

  async function handleDetect() {
    patch({ detectLoading: true });
    try {
      const result = await api.post<IPDetectResult>(`/nodes/${node.id}/ip-sentinel/detect`, {});
      patch({ detectResult: result });
      toast(`检测完成：${result.ip}（${result.country} · ${result.city}）`, "success");
      fetchRuns();
    } catch (err) {
      handleError(err, "检测失败");
    } finally {
      patch({ detectLoading: false });
    }
  }

  // ── 立即执行 ─────────────────────────────────────────────────────

  async function handleRun() {
    patch({ runLoading: true });
    try {
      await api.post<{ ok: boolean; run_id: string }>(`/nodes/${node.id}/ip-sentinel/run`, {});
      toast("任务已提交，正在轮询结果…", "success");
      startPolling();
    } catch (err) {
      handleError(err, "提交失败");
    } finally {
      patch({ runLoading: false });
    }
  }

  // ── 保存配置 ─────────────────────────────────────────────────────

  async function handleSaveConfig() {
    patch({ configSaving: true });
    try {
      await api.put<{ ok: boolean }>(`/nodes/${node.id}/ip-sentinel/config`, {
        region_code: state.draftCode,
        region_name: state.draftName,
      });
      patch({
        config: { region_code: state.draftCode, region_name: state.draftName },
      });
      toast("地区设置已保存", "success");
    } catch (err) {
      handleError(err, "保存失败");
    } finally {
      patch({ configSaving: false });
    }
  }

  // ── 选择预设地区 ─────────────────────────────────────────────────

  function handlePreset(code: string) {
    const preset = REGION_PRESETS.find((p) => p.code === code);
    if (preset) {
      patch({ draftCode: preset.code, draftName: preset.name });
    }
  }

  // ── 衍生数据 ─────────────────────────────────────────────────────

  const latestRun = state.runs[0] ?? null;
  const recentRuns = state.runs.slice(0, 5);
  const isPreset = REGION_PRESETS.some((p) => p.code === state.draftCode);

  return (
    <Card className="flex flex-col">
      {/* 卡头 */}
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="text-sm font-semibold leading-snug">{node.name}</CardTitle>
          {/* 最近运行状态 badge */}
          <span
            className={`shrink-0 inline-flex items-center rounded px-2 py-0.5 text-[11px] font-medium ${statusBadgeClass(latestRun?.status)}`}
          >
            {latestRun ? (
              <>
                {(latestRun.status === "pending" || latestRun.status === "running") && (
                  <SpinIcon className="h-2.5 w-2.5 mr-1 animate-spin" />
                )}
                {statusLabel(latestRun.status)}
              </>
            ) : (
              "--"
            )}
          </span>
        </div>
        {/* IP 行 */}
        {state.detectResult ? (
          <div className="flex items-center gap-1.5 mt-1 flex-wrap">
            <span className="font-mono text-xs font-medium">{state.detectResult.ip}</span>
            <span className="text-[11px] text-[hsl(var(--muted-foreground))]">
              {state.detectResult.country_code} · {state.detectResult.city}
            </span>
          </div>
        ) : (
          <div className="text-[11px] text-[hsl(var(--muted-foreground))] mt-1">-- 未检测</div>
        )}
      </CardHeader>

      <CardContent className="flex flex-col gap-4 flex-1">
        {/* 地区设置行 */}
        <div className="space-y-2">
          <Label className="text-xs text-[hsl(var(--muted-foreground))]">地区设置</Label>
          <Select
            value={isPreset ? state.draftCode : undefined}
            onValueChange={(v) => handlePreset(v)}
          >
            <SelectTrigger className="h-8 text-xs">
              <SelectValue placeholder="— 选择预设地区 —" />
            </SelectTrigger>
            <SelectContent>
              {REGION_PRESETS.map((p) => (
                <SelectItem key={p.code} value={p.code} className="text-xs">
                  {p.code} — {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="grid grid-cols-2 gap-2">
            <Input
              placeholder="代码（如 US）"
              value={state.draftCode}
              onChange={(e) => patch({ draftCode: e.target.value.toUpperCase() })}
              className="h-8 text-xs"
            />
            <Input
              placeholder="名称（如 United States）"
              value={state.draftName}
              onChange={(e) => patch({ draftName: e.target.value })}
              className="h-8 text-xs"
            />
          </div>
          <div className="flex justify-end">
            <Button
              size="sm"
              variant="outline"
              disabled={state.configSaving || !state.configLoaded}
              onClick={handleSaveConfig}
              className="h-7 text-xs px-3"
            >
              {state.configSaving ? "保存中…" : "保存"}
            </Button>
          </div>
        </div>

        {/* 操作行 */}
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={state.detectLoading}
            onClick={handleDetect}
            className="h-7 text-xs px-3 gap-1"
          >
            {state.detectLoading ? (
              <SpinIcon className="h-3 w-3 animate-spin" />
            ) : (
              <RadarIcon className="h-3 w-3" />
            )}
            检测 IP
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={state.runLoading}
            onClick={handleRun}
            className="h-7 text-xs px-3 gap-1"
          >
            {state.runLoading ? (
              <SpinIcon className="h-3 w-3 animate-spin" />
            ) : (
              <PlayIcon className="h-3 w-3" />
            )}
            立即执行
          </Button>
        </div>

        {/* 执行记录（展开/折叠） */}
        <div>
          <button
            className="flex w-full items-center justify-between py-1 text-xs text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))] transition-colors"
            onClick={() => patch({ runsExpanded: !state.runsExpanded })}
          >
            <span className="font-medium">最近执行记录</span>
            <ChevronIcon
              className={`h-3.5 w-3.5 transition-transform ${state.runsExpanded ? "rotate-180" : ""}`}
            />
          </button>

          {state.runsExpanded && (
            <div className="mt-1.5 rounded-md border border-[hsl(var(--border))] overflow-hidden">
              {!state.runsLoaded ? (
                <div className="py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">
                  加载中…
                </div>
              ) : recentRuns.length === 0 ? (
                <div className="py-3 text-center text-xs text-[hsl(var(--muted-foreground))]">
                  暂无记录
                </div>
              ) : (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-[hsl(var(--border))] bg-[hsl(var(--muted))]">
                      <th className="py-1.5 pl-3 pr-2 text-left text-[10px] font-medium text-[hsl(var(--muted-foreground))]">时间</th>
                      <th className="py-1.5 pr-2 text-left text-[10px] font-medium text-[hsl(var(--muted-foreground))]">触发</th>
                      <th className="py-1.5 pr-2 text-left text-[10px] font-medium text-[hsl(var(--muted-foreground))]">状态</th>
                      <th className="py-1.5 pr-3 text-right text-[10px] font-medium text-[hsl(var(--muted-foreground))]">耗时</th>
                    </tr>
                  </thead>
                  <tbody>
                    {recentRuns.map((run) => (
                      <RunRow key={run.id} run={run} />
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// ── 全局 Schedule 设置条 ──────────────────────────────────────────

function ScheduleBar({ onAuthError }: { onAuthError: () => void }) {
  const [schedule, setSchedule] = useState<Schedule | null>(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [runningAll, setRunningAll] = useState(false);

  const fetchSchedule = useCallback(async () => {
    try {
      const data = await api.get<Schedule>("/ip-sentinel/schedule");
      setSchedule(data);
    } catch (err) {
      if (err instanceof AuthError) {
        onAuthError();
      }
      // 静默失败，不影响主体
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    fetchSchedule();
  }, [fetchSchedule]);

  async function handleSave() {
    const hours = parseInt(draft, 10);
    if (isNaN(hours) || hours < 1) {
      toast("请输入有效的小时数（≥1）", "error");
      return;
    }
    setSaving(true);
    try {
      const res = await api.put<{ ok: boolean; interval_hours: number }>(
        "/ip-sentinel/schedule",
        { interval_hours: hours }
      );
      setSchedule((prev) =>
        prev ? { ...prev, interval_hours: res.interval_hours } : null
      );
      setEditing(false);
      toast(`执行间隔已更新为 ${res.interval_hours} 小时`, "success");
    } catch (err) {
      if (err instanceof AuthError) {
        onAuthError();
      } else {
        toast(err instanceof Error ? err.message : "保存失败", "error");
      }
    } finally {
      setSaving(false);
    }
  }

  function startEdit() {
    setDraft(String(schedule?.interval_hours ?? "1"));
    setEditing(true);
  }

  function cancelEdit() {
    setEditing(false);
  }

  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-4 py-3 text-sm">
      {/* 执行间隔 */}
      <div className="flex items-center gap-2">
        <ClockIcon className="h-4 w-4 text-[hsl(var(--muted-foreground))]" />
        <span className="text-[hsl(var(--muted-foreground))]">执行间隔</span>
        {editing ? (
          <div className="flex items-center gap-1.5">
            <Input
              type="number"
              min={1}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleSave();
                if (e.key === "Escape") cancelEdit();
              }}
              className="h-7 w-20 text-xs"
              autoFocus
            />
            <span className="text-[hsl(var(--muted-foreground))] text-xs">小时</span>
            <Button
              size="sm"
              disabled={saving}
              onClick={handleSave}
              className="h-7 text-xs px-2"
            >
              {saving ? "保存…" : "确定"}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={cancelEdit}
              className="h-7 text-xs px-2"
            >
              取消
            </Button>
          </div>
        ) : (
          <button
            className="flex items-center gap-1 font-medium hover:text-[hsl(var(--primary))] transition-colors"
            onClick={startEdit}
            title="点击编辑"
          >
            {schedule ? `${schedule.interval_hours} 小时` : "--"}
            <PencilIcon className="h-3 w-3 opacity-60" />
          </button>
        )}
      </div>

      {/* 分隔线 */}
      <div className="h-4 w-px bg-[hsl(var(--border))] hidden sm:block" />

      {/* 上次执行 */}
      <div className="flex items-center gap-1.5 text-xs text-[hsl(var(--muted-foreground))]">
        <span>上次执行：</span>
        <span className="font-medium text-[hsl(var(--foreground))]">
          {formatTime(schedule?.last_run_at)}
        </span>
      </div>

      {/* 下次执行 */}
      <div className="flex items-center gap-1.5 text-xs text-[hsl(var(--muted-foreground))]">
        <span>下次执行：</span>
        <span className="font-medium text-[hsl(var(--foreground))]">
          {formatTime(schedule?.next_run_at)}
        </span>
      </div>

      {/* 分隔线 */}
      <div className="h-4 w-px bg-[hsl(var(--border))] hidden sm:block" />

      {/* 立即全部执行 */}
      <Button
        size="sm"
        variant="outline"
        disabled={runningAll}
        className="h-7 text-xs px-3"
        onClick={async () => {
          setRunningAll(true);
          try {
            await api.post("/ip-sentinel/run-all", {});
            toast("已触发全部节点执行，请稍候查看各节点状态", "success");
            setTimeout(fetchSchedule, 3000);
          } catch (err) {
            if (err instanceof AuthError) onAuthError();
            else toast("触发失败", "error");
          } finally {
            setRunningAll(false);
          }
        }}
      >
        {runningAll ? "触发中…" : "立即全部执行"}
      </Button>
    </div>
  );
}

// ── 主页面 ───────────────────────────────────────────────────────

export default function IPSentinelPage() {
  const navigate = useNavigate();
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [loading, setLoading] = useState(true);

  function handleAuthError() {
    clearToken();
    navigate({ to: "/panel/login" });
  }

  const fetchNodes = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.get<NodesResponse>("/nodes");
      setNodes(data.nodes ?? []);
    } catch (err) {
      if (err instanceof AuthError) {
        handleAuthError();
      } else {
        toast(err instanceof Error ? err.message : "加载节点列表失败", "error");
      }
    } finally {
      setLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    fetchNodes();
  }, [fetchNodes]);

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <div className="max-w-6xl w-full mx-auto px-6 py-6 space-y-6">
        {/* 页面标题 */}
        <div className="flex items-center gap-3">
          <ShieldIcon className="h-5 w-5 text-[hsl(var(--primary))]" />
          <div>
            <h1 className="text-lg font-semibold leading-none">Sentinel</h1>
            <p className="text-xs text-[hsl(var(--muted-foreground))] mt-1">
              自动化 IP 纠偏系统，定时对所有节点执行检测与修正
            </p>
          </div>
        </div>

        {/* 全局设置条 */}
        <ScheduleBar onAuthError={handleAuthError} />

        {/* 节点卡片网格 */}
        {loading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
            {[...Array(3)].map((_, i) => (
              <div
                key={i}
                className="rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--card))] h-64 animate-pulse"
              />
            ))}
          </div>
        ) : nodes.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-[hsl(var(--muted-foreground))] gap-3">
            <ShieldIcon className="h-10 w-10 opacity-20" />
            <p className="text-sm">暂无节点，请先在"节点"页面添加节点</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
            {nodes.map((node) => (
              <NodeCard
                key={node.id}
                node={node}
                onAuthError={handleAuthError}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// ── 内联 SVG 图标 ─────────────────────────────────────────────────

function SpinIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M21 12a9 9 0 1 1-6.219-8.56" />
    </svg>
  );
}

function RadarIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M19.07 4.93A10 10 0 0 0 6.99 3.34" />
      <path d="M4 6h.01" />
      <path d="M2.29 9.62A10 10 0 1 0 21.31 8.35" />
      <path d="M16.24 7.76A6 6 0 1 0 8.23 16.67" />
      <line x1="12" y1="12" x2="12" y2="12.01" />
    </svg>
  );
}

function PlayIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <polygon points="5 3 19 12 5 21 5 3" />
    </svg>
  );
}

function ClockIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <circle cx="12" cy="12" r="10" />
      <polyline points="12 6 12 12 16 14" />
    </svg>
  );
}

function PencilIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
      <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
    </svg>
  );
}

function ShieldIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    </svg>
  );
}

function ChevronIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <polyline points="6 9 12 15 18 9" />
    </svg>
  );
}
