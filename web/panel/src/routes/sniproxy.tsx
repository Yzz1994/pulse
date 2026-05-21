import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
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
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui";
import { ScrollArea } from "@/components/ui/scroll-area";
import { api } from "@/lib/api";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";
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

const modeKeys: Record<SNIRoute["mode"], string> = {
  terminating: "sniproxy.tlsTerminate",
  "http-reverse": "sniproxy.httpReverse",
  transparent: "sniproxy.transparent",
};

// ── HTTP 反代规则编辑器 ───────────────────────────────────────────

interface HTTPReverseRulesProps {
  node: Node;
  onNodeUpdated: (node: Node) => void;
}

function HTTPReverseRules({ node, onNodeUpdated }: HTTPReverseRulesProps) {
  const { t } = useTranslation();
  const handleAuthError = useAuthErrorHandler();
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
      toast(t("sniproxy.httpRulesSaved"), "success");
      setDirty(false);
      onNodeUpdated(updated);
    } catch (err) {
      if (!handleAuthError(err)) {
        toast(err instanceof Error ? err.message : t("common.saveFailed"), "error");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold uppercase tracking-wide text-[hsl(var(--muted-foreground))]">
          {t("sniproxy.httpRulesTitle")}
        </p>
        {dirty && (
          <Button
            size="sm"
            className="h-6 px-2 text-xs"
            onClick={save}
            disabled={saving}
          >
            {saving ? t("common.saving") : t("sniproxy.saveAndSync")}
          </Button>
        )}
      </div>

      {rules.length > 0 && (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-xs">{t("sniproxy.domain")}</TableHead>
                <TableHead className="text-xs">{t("sniproxy.backend")}</TableHead>
                <TableHead className="w-10 text-xs" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((r, i) => (
                <TableRow key={i}>
                  <TableCell className="py-1.5 font-mono text-xs">{r.domain}</TableCell>
                  <TableCell className="py-1.5 font-mono text-xs">{r.backend}</TableCell>
                  <TableCell className="py-1.5">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-6 w-6 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--destructive))]"
                      onClick={() => removeRule(i)}
                    >
                      ✕
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <div className="flex items-center gap-2">
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
          {t("common.add")}
        </Button>
      </div>
    </div>
  );
}

// ── 主页面 ────────────────────────────────────────────────────────

export default function SNIProxyPage() {
  const { t } = useTranslation();
  const handleAuthError = useAuthErrorHandler();
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
        toast(err instanceof Error ? err.message : t("sniproxy.loadNodesFailed"), "error");
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
      toast(`${node.name} ${t("sniproxy.syncTriggered")}`, "success");
      setTimeout(() => fetchStatus(node), 2000);
    } catch (err) {
      if (!handleAuthError(err)) {
        toast(err instanceof Error ? err.message : t("sniproxy.syncFailed"), "error");
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
            {t("sniproxy.desc")}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={refreshAll} disabled={refreshing}>
          {refreshing ? t("common.refreshing") : t("sniproxy.refreshAll")}
        </Button>
      </div>

      {loadingNodes && (
        <p className="text-sm text-[hsl(var(--muted-foreground))]">{t("sniproxy.loadNodes")}</p>
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
                  <Badge variant="default">{t("sniproxy.running")} {row.resp.status.listen}</Badge>
                ) : row?.resp ? (
                  <Badge variant="secondary">{t("sniproxy.notManaged")}</Badge>
                ) : row?.error ? (
                  <Badge variant="destructive">{t("sniproxy.unreachable")}</Badge>
                ) : (
                  <Badge variant="outline">{t("sniproxy.loading")}</Badge>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => fetchStatus(node)}
                  disabled={row?.loading}
                >
                  {t("common.refresh")}
                </Button>
                <Button variant="outline" size="sm" onClick={() => sync(node)}>
                  {t("sniproxy.syncBtn")}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {row?.error && (
                <div className="rounded-md border border-[hsl(var(--destructive)/0.3)] bg-[hsl(var(--destructive)/0.1)] px-3 py-2 text-xs text-[hsl(var(--destructive))]">
                  {row.error}
                </div>
              )}
              {row?.resp?.status.last_error && (
                <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-yellow-600 dark:text-yellow-400">
                  <span className="font-semibold">LastError：</span>
                  <span className="font-mono">{row.resp.status.last_error}</span>
                </div>
              )}
              {row?.resp && (
                <>
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="inline-flex items-center gap-1 rounded-full bg-[hsl(var(--muted))] px-2 py-0.5 text-xs text-[hsl(var(--muted-foreground))]">
                      {t("sniproxy.routes")} {row.resp.status.route_count} {t("sniproxy.count")}
                    </span>
                    <span className="inline-flex items-center gap-1 rounded-full bg-[hsl(var(--muted))] px-2 py-0.5 text-xs text-[hsl(var(--muted-foreground))]">
                      {t("sniproxy.certificates")} {row.resp.status.cert_domains} {t("sniproxy.sheets")}
                    </span>
                    {row.resp.status.storage_path && (
                      <span className="inline-flex items-center gap-1 text-xs font-mono text-[hsl(var(--muted-foreground))]">
                        <svg
                          xmlns="http://www.w3.org/2000/svg"
                          viewBox="0 0 16 16"
                          fill="currentColor"
                          className="h-4 w-4 shrink-0"
                        >
                          <path d="M2 3.5A1.5 1.5 0 0 1 3.5 2h3.879a1.5 1.5 0 0 1 1.06.44l.622.621A1.5 1.5 0 0 0 10.12 3.5H12.5A1.5 1.5 0 0 1 14 5v1H2V3.5ZM2 7.5h12v5A1.5 1.5 0 0 1 12.5 14h-9A1.5 1.5 0 0 1 2 12.5v-5Z" />
                        </svg>
                        {row.resp.status.storage_path}
                      </span>
                    )}
                  </div>

                  {row.resp.status.routes && row.resp.status.routes.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="text-xs font-semibold uppercase tracking-wide text-[hsl(var(--muted-foreground))]">
                        {t("sniproxy.routeTable")}
                      </p>
                      <ScrollArea className="max-h-48 rounded-md border">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead className="text-xs">{t("sniproxy.sni")}</TableHead>
                              <TableHead className="text-xs">{t("sniproxy.mode")}</TableHead>
                              <TableHead className="text-xs">{t("sniproxy.backend")}</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {row.resp.status.routes.map((r, i) => (
                              <TableRow key={i}>
                                <TableCell className="py-1.5 font-mono text-xs">{r.sni}</TableCell>
                                <TableCell className="py-1.5">
                                  <Badge
                                    variant={
                                      r.mode === "terminating"
                                        ? "default"
                                        : r.mode === "http-reverse"
                                          ? "outline"
                                          : "secondary"
                                    }
                                    className="text-xs"
                                  >
                                    {t(modeKeys[r.mode]) ?? r.mode}
                                  </Badge>
                                </TableCell>
                                <TableCell className="py-1.5 font-mono text-xs">{r.backend}</TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </ScrollArea>
                    </div>
                  )}

                  {row.resp.status.certs && row.resp.status.certs.length > 0 && (
                    <div className="space-y-1.5">
                      <p className="text-xs font-semibold uppercase tracking-wide text-[hsl(var(--muted-foreground))]">
                        {t("sniproxy.certTable")}
                      </p>
                      <ScrollArea className="max-h-48 rounded-md border">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead className="text-xs">{t("sniproxy.domain")}</TableHead>
                              <TableHead className="text-xs">{t("sniproxy.status")}</TableHead>
                              <TableHead className="text-xs">{t("sniproxy.expiry")}</TableHead>
                              <TableHead className="text-xs">{t("sniproxy.issuer")}</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {row.resp.status.certs.map((c, i) => {
                              const days = daysRemaining(c.not_after);
                              const expiringSoon = days !== null && days < 30;
                              return (
                                <TableRow key={i}>
                                  <TableCell className="py-1.5 font-mono text-xs">{c.domain}</TableCell>
                                  <TableCell className="py-1.5">
                                    {c.ready ? (
                                      <Badge
                                        variant="outline"
                                        className="border-green-500/40 text-xs text-green-600 dark:text-green-400"
                                      >
                                        {t("sniproxy.ready")}
                                      </Badge>
                                    ) : (
                                      <Badge variant="destructive" className="text-xs">
                                        {t("sniproxy.notReady")}
                                      </Badge>
                                    )}
                                  </TableCell>
                                  <TableCell className="py-1.5 text-xs">
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
                                  </TableCell>
                                  <TableCell className="py-1.5 text-xs text-[hsl(var(--muted-foreground))]">
                                    {c.issuer ?? "—"}
                                  </TableCell>
                                </TableRow>
                              );
                            })}
                          </TableBody>
                        </Table>
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
