import { useEffect, useState } from "react";
import { api } from "@/lib/api";

// 与后端 internal/serverapi/nodehub_metrics.go 对齐
export interface NodeHubMetrics {
  online_count: number;
  online_node_ids: string[];
  calls_total: number;
  calls_err_total: number;
  calls_offline_total: number;
  push_usage_total: number;
  push_usage_ack_total: number;
  reconnect_total: number;
  call_latency_avg_ns: number;
  last_seen: Record<string, number>; // unix seconds
  enroll_success_total: number;
  enroll_failure_total: number;
}

/**
 * useNodeHubMetrics 每隔 intervalMs 轮询 /v1/system/nodehub/metrics。
 *
 * 端点尚未启用（hub 未实例化）时返回 404，hook 静默吞掉错误并
 * 返回 null，让调用方按"全部离线"处理即可。
 */
export function useNodeHubMetrics(intervalMs = 5000): NodeHubMetrics | null {
  const [metrics, setMetrics] = useState<NodeHubMetrics | null>(null);

  useEffect(() => {
    let cancelled = false;
    const tick = () => {
      api
        .get<NodeHubMetrics>("/system/nodehub/metrics")
        .then((m) => {
          if (!cancelled) setMetrics(m);
        })
        .catch(() => {
          if (!cancelled) setMetrics(null);
        });
    };
    tick();
    const id = window.setInterval(tick, intervalMs);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [intervalMs]);

  return metrics;
}

/** 把"5 秒前"渲染成中文相对时间。 */
export function formatLastSeen(unixSec: number | undefined): string {
  if (!unixSec) return "未知";
  const diff = Math.floor(Date.now() / 1000 - unixSec);
  if (diff < 0) return "刚刚";
  if (diff < 60) return `${diff} 秒前`;
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
  return `${Math.floor(diff / 86400)} 天前`;
}
