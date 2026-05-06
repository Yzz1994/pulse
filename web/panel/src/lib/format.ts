/**
 * Format a byte count into a human-readable string.
 * e.g. 1536 → "1.5 KB", 1073741824 → "1 GB"
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
  const idx = Math.min(i, sizes.length - 1);
  return parseFloat((bytes / Math.pow(k, idx)).toFixed(2)) + " " + sizes[idx];
}

/**
 * Format bytes/s into a human-readable speed string.
 * e.g. 1536 → "1.5 KB/s", 0 → "0 B/s"
 */
export function formatSpeed(bps: number): string {
  if (bps === 0) return "0 B/s";
  const k = 1024;
  const sizes = ["B/s", "KB/s", "MB/s", "GB/s"];
  const i = Math.min(Math.floor(Math.log(bps) / Math.log(k)), sizes.length - 1);
  return parseFloat((bps / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

/**
 * 生成 inbound 的附加标注后缀（紧跟在名称后面，空格分隔）。
 * 规则（按顺序拼接，均为可选）：
 *   - traffic_rate != 1 → [1.5x]
 * 后续在此处扩展新规则即可。
 */
export function inboundBadge(ib: { traffic_rate?: number }): string {
  const parts: string[] = [];
  if (ib.traffic_rate != null && ib.traffic_rate !== 1) {
    const r = parseFloat(ib.traffic_rate.toFixed(2));
    parts.push(`[${r}x]`);
  }
  return parts.length ? " " + parts.join(" ") : "";
}

/**
 * 根据 host 字段计算订阅显示名（镜像后端 buildName 逻辑）。
 * 优先使用 remark，其次拼接 country/region/network/entry，最后追加倍率标注。
 */
export function hostSubName(
  host: { remark?: string; country?: string; region?: string; network?: string; entry?: string; tags?: string },
  trafficRate?: number
): string {
  let name = "";
  if (host.remark) {
    name = host.remark;
  } else {
    const parts = [host.country, host.region, host.network, host.entry, host.tags].filter(Boolean);
    name = parts.join(" ");
  }
  if (trafficRate != null && trafficRate !== 0 && trafficRate !== 1) {
    const r = parseFloat(trafficRate.toFixed(2));
    name = name.replace(/\s*\[\d+\.?\d*x\]$/, "") + ` [${r}x]`;
  }
  return name;
}

/**
 * Compact byte formatting for chart axis labels.
 * e.g. 1073741824 → "1 GB"
 */
export function formatBytesCompact(bytes: number): string {
  if (bytes === 0) return "0";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
  const idx = Math.min(i, sizes.length - 1);
  const val = bytes / Math.pow(k, idx);
  return (val % 1 === 0 ? val.toString() : val.toFixed(1)) + " " + sizes[idx];
}
