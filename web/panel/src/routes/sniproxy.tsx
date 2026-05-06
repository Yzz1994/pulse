import { useEffect, useState, useCallback } from "react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
  Badge,
  Button,
  Input,
} from "@/components/ui";
import { ScrollArea } from "@/components/ui/scroll-area";
import { api, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";
import { toast } from "@/components/ui";
import type { Node, NodesResponse } from "@/lib/types";

// 节点侧 sniproxy.Manager.Status 的 JSON 结构。字段随后端演进，这里只用我们关心的。
interface SNIRoute {
  sni: string;
  backend: string;
  mode: "transparent" | "terminating" | "http-reverse";
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
  storage_path?: string;
}
interface SNIStatusResponse {
  enabled: boolean;
  status: SNIStatus;
  config: {
    listen: string;
    acme_email?: string;
    acme_staging?: boolean;
  };
}

// 每个节点的组合视图
interface NodeStatus {
  node: Node;
  loading: boolean;
  error?: string;
  resp?: SNIStatusResponse;
}

interface ProxyRule {
  domain: string;
  backend: string;
}

function handleAuthError(err: unknown): boolean {
  if (err instanceof AuthError) {
    clearToken();
    window.location.href = "/panel/login";
    return true;
  }
  return false;
}

function fmtDate(s?: string): string {
  if (!s) return "—";
  const d = new Date(s);
  if (isNaN(d.getTime())) return s;
  return d.toISOString().slice(0, 10);
}

function daysRemaining(s?: string): number | null {
  if (!s) return null;
  const d = new Date(s);
  if (isNaN(d.getTime())) return null;
  const diff = d.getTime() - Date.now();
  return Math.floor(diff / 86_400_000);
}

// 解析 extra_proxies：
//   新格式："domain host:port"（空格分隔）
//   旧格式（兼容）："domain:port"
function parseProxyRules(raw: string): ProxyRule[] {
  return raw
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#"))
    .flatMap((l) => {
      const spaceIdx = l.indexOf(" ");
      if (spaceIdx > 0) {
        const domain = l.slice(0, spaceIdx).trim();
        const backend = l.slice(spaceIdx + 1).trim();
        return domain && backend ? [{ domain, backend }] : [];
      }
      const colonIdx = l.lastIndexOf(":");
      if (colonIdx <= 0) return [];
      const domain = l.slice(0, colonIdx).trim();
      const port = l.slice(colonIdx + 1).trim();
      return domain && port ? [{ domain, backend: `127.0.0.1:${port}` }] : [];
    });
}

function serializeProxyRules(rules: ProxyRule[]): string {
  return rules.map((r) => `${r.domain} ${r.backend}`).join("\n");
}

// ── HTTP 反代规则编辑器 ───────────────────────────────────────────

interface HTTPReverseRulesProps {
  node: Node;
  onNodeUpdated: (node: Node) => void;
}

function HTTPReverseRules({ node, onNodeUpdated }: HTTPReverseRulesProps) {
  const [rules, setRules] = useState<ProxyRule[]>(() =>
    parseProxyRules(node.extra_proxies ?? "")
  );
  const [addDomain, setAddDomain] = useState("");
  const [addBackend, setAddBackend] = useState("");
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    setRules(parseProxyRules(node.extra_proxies ?? ""));
    setDirty(false);
  }, [node.extra_proxies]);

  const addRule = () => {
    const d = addDomain.trim();
    const b = addBackend.trim();
    if (!d || !b) return;
    setRules((prev) => [...prev, { domain: d, backend: b }]);
    setAddDomain("");
    setAddBackend("");
    setDirty(true);
  };

  const removeRule = (idx: number) => {
    setRules((prev) => prev.filter((_, i) => i !== idx));
    setDirty(true);
  };

  const save = async () => {
    setSaving(true);
    try {
      const updated = await api.put<Node>(`/nodes/${node.id}`, {
        ...node,
        extra_proxies: serializeProxyRules(rules),
      });
      await api.post(`/nodes/${node.id}/sniproxy/sync`, {});
      toast("HTTP 反代规则已保存并同步", "success");
      setDirty(false);
      onNodeUpdated(updated);
    } catch (err) {
      if (!handleAuthError(err)) {
        toast(err instanceof Error ? err.message : "保存失败", "error");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-xs font-semibold">HTTP 反代规则</span>
        {dirty && (
          <Button
            size="sm"
            className="h-6 px-2 text-xs"
            onClick={save}
            disabled={saving}
          >
            {saving ? "保存中…" : "保存并同步"}
          </Button>
        )}
      </div>

      <div className="rounded border text-xs">
        {rules.length > 0 && (
          <table className="w-full">
            <thead className="bg-[hsl(var(--muted))] text-left">
              <tr>
                <th className="px-2 py-1">域名</th>
                <th className="px-2 py-1">后端</th>
                <th className="w-8 px-2 py-1" />
              </tr>
            </thead>
            <tbody>
              {rules.map((r, i) => (
                <tr key={i} className="border-t">
                  <td className="px-2 py-1 font-mono">{r.domain}</td>
                  <td className="px-2 py-1 font-mono">{r.backend}</td>
                  <td className="px-2 py-1">
                    <button
                      className="text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--destructive))]"
                      onClick={() => removeRule(i)}
                    >
                      ✕
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        <div
          className={`flex items-center gap-2 p-2 ${rules.length > 0 ? "border-t" : ""}`}
        >
          <Input
            className="h-7 flex-1 font-mono text-xs"
            placeholder="panel.example.com"
            value={addDomain}
            onChange={(e) => setAddDomain(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addBackend && addRule()}
          />
          <span className="shrink-0 text-[hsl(var(--muted-foreground))]">→</span>
          <Input
            className="h-7 flex-1 font-mono text-xs"
            placeholder="1.2.3.4:8080"
            value={addBackend}
            onChange={(e) => setAddBackend(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addDomain && addRule()}
          />
          <Button
            variant="outline"
            size="sm"
            className="h-7 shrink-0 px-2 text-xs"
            onClick={addRule}
            disabled={!addDomain.trim() || !addBackend.trim()}
          >
            添加
          </Button>
        </div>
      </div>
    </div>
  );
}

// ── 主页面 ────────────────────────────────────────────────────────

export default function SNIProxyPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [rows, setRows] = useState<Map<string, NodeStatus>>(new Map());
  const [loadingNodes, setLoadingNodes] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const fetchNodes = useCallback(async () => {
    setLoadingNodes(true);
    try {
      const r = await api.get<NodesResponse>("/nodes");
      setNodes(r.nodes ?? []);
    } catch (err) {
      if (!handleAuthError(err)) {
        toast(err instanceof Error ? err.message : "加载节点失败", "error");
      }
    } finally {
      setLoadingNodes(false);
    }
  }, []);

  const fetchStatus = useCallback(async (node: Node) => {
    setRows((m) => new Map(m).set(node.id, { node, loading: true }));
    try {
      const resp = await api.get<SNIStatusResponse>(`/nodes/${node.id}/sniproxy/status`);
      setRows((m) => new Map(m).set(node.id, { node, loading: false, resp }));
    } catch (err) {
      if (handleAuthError(err)) return;
      const msg = err instanceof Error ? err.message : String(err);
      setRows((m) => new Map(m).set(node.id, { node, loading: false, error: msg }));
    }
  }, []);

  useEffect(() => {
    fetchNodes();
  }, [fetchNodes]);

  useEffect(() => {
    nodes.forEach((n) => fetchStatus(n));
  }, [nodes, fetchStatus]);

  const handleNodeUpdated = useCallback((updated: Node) => {
    setNodes((prev) => prev.map((n) => (n.id === updated.id ? updated : n)));
  }, []);

  const sync = async (node: Node) => {
    try {
      await api.post(`/nodes/${node.id}/sniproxy/sync`, {});
      toast(`${node.name} 同步已触发`, "success");
      setTimeout(() => fetchStatus(node), 2000);
    } catch (err) {
      if (!handleAuthError(err)) {
        toast(err instanceof Error ? err.message : "同步失败", "error");
      }
    }
  };

  const refreshAll = async () => {
    setRefreshing(true);
    try {
      await Promise.all(nodes.map(fetchStatus));
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <div className="mx-auto max-w-5xl space-y-4 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">NodeGate</h1>
          <p className="text-sm text-[hsl(var(--muted-foreground))]">
            节点内置 NodeGate（SNI 代理）运行状态、路由表和证书明细。
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={refreshAll} disabled={refreshing}>
          {refreshing ? "刷新中…" : "全部刷新"}
        </Button>
      </div>

      {loadingNodes && (
        <p className="text-sm text-[hsl(var(--muted-foreground))]">加载节点列表…</p>
      )}

      {nodes.map((node) => {
        const row = rows.get(node.id);
        return (
          <Card key={node.id}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <div>
                <CardTitle className="text-base">{node.name}</CardTitle>
                <CardDescription className="font-mono text-xs">{node.base_url}</CardDescription>
              </div>
              <div className="flex items-center gap-2">
                {row?.resp?.enabled ? (
                  <Badge variant="default">运行中 {row.resp.status.listen}</Badge>
                ) : row?.resp ? (
                  <Badge variant="secondary">未接管</Badge>
                ) : row?.error ? (
                  <Badge variant="destructive">节点不可达</Badge>
                ) : (
                  <Badge variant="outline">加载…</Badge>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => fetchStatus(node)}
                  disabled={row?.loading}
                >
                  刷新
                </Button>
                <Button variant="outline" size="sm" onClick={() => sync(node)}>
                  同步
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              {row?.error && (
                <div className="rounded border border-[hsl(var(--destructive))] p-2 text-xs text-[hsl(var(--destructive))]">
                  {row.error}
                </div>
              )}
              {row?.resp?.status.last_error && (
                <div className="rounded border border-yellow-500 p-2 text-xs">
                  <span className="font-semibold">LastError：</span>
                  <span className="font-mono">{row.resp.status.last_error}</span>
                </div>
              )}
              {row?.resp && (
                <>
                  <div className="flex gap-6 text-xs text-[hsl(var(--muted-foreground))]">
                    <span>路由 {row.resp.status.route_count} 条</span>
                    <span>证书 {row.resp.status.cert_domains} 张</span>
                    {row.resp.status.storage_path && (
                      <span className="font-mono">📁 {row.resp.status.storage_path}</span>
                    )}
                  </div>

                  {row.resp.status.routes && row.resp.status.routes.length > 0 && (
                    <div>
                      <div className="mb-1 text-xs font-semibold">路由表</div>
                      <ScrollArea className="max-h-48 rounded border">
                        <table className="w-full text-xs">
                          <thead className="bg-[hsl(var(--muted))] text-left">
                            <tr>
                              <th className="px-2 py-1">SNI</th>
                              <th className="px-2 py-1">模式</th>
                              <th className="px-2 py-1">后端</th>
                            </tr>
                          </thead>
                          <tbody>
                            {row.resp.status.routes.map((r, i) => (
                              <tr key={i} className="border-t">
                                <td className="px-2 py-1 font-mono">{r.sni}</td>
                                <td className="px-2 py-1">
                                  <Badge
                                    variant={
                                      r.mode === "terminating"
                                        ? "default"
                                        : r.mode === "http-reverse"
                                          ? "outline"
                                          : "secondary"
                                    }
                                  >
                                    {r.mode}
                                  </Badge>
                                </td>
                                <td className="px-2 py-1 font-mono">{r.backend}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </ScrollArea>
                    </div>
                  )}

                  {row.resp.status.certs && row.resp.status.certs.length > 0 && (
                    <div>
                      <div className="mb-1 text-xs font-semibold">证书</div>
                      <ScrollArea className="max-h-48 rounded border">
                        <table className="w-full text-xs">
                          <thead className="bg-[hsl(var(--muted))] text-left">
                            <tr>
                              <th className="px-2 py-1">域名</th>
                              <th className="px-2 py-1">状态</th>
                              <th className="px-2 py-1">到期</th>
                              <th className="px-2 py-1">签发者</th>
                            </tr>
                          </thead>
                          <tbody>
                            {row.resp.status.certs.map((c, i) => {
                              const days = daysRemaining(c.not_after);
                              const expiringSoon = days !== null && days < 30;
                              return (
                                <tr key={i} className="border-t">
                                  <td className="px-2 py-1 font-mono">{c.domain}</td>
                                  <td className="px-2 py-1">
                                    {c.ready ? (
                                      <Badge variant="default">就绪</Badge>
                                    ) : (
                                      <Badge variant="destructive">未就绪</Badge>
                                    )}
                                  </td>
                                  <td className="px-2 py-1">
                                    {fmtDate(c.not_after)}
                                    {days !== null && (
                                      <span
                                        className={`ml-1 text-[10px] ${
                                          expiringSoon
                                            ? "text-[hsl(var(--destructive))]"
                                            : "text-[hsl(var(--muted-foreground))]"
                                        }`}
                                      >
                                        ({days}d)
                                      </span>
                                    )}
                                  </td>
                                  <td className="px-2 py-1 text-[hsl(var(--muted-foreground))]">
                                    {c.issuer ?? "—"}
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </ScrollArea>
                    </div>
                  )}
                </>
              )}

              {/* HTTP 反代规则编辑器：始终显示，无论 sniproxy 是否已接管 */}
              <HTTPReverseRules node={node} onNodeUpdated={handleNodeUpdated} />
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
