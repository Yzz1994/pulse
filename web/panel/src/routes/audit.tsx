import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  Button,
  Input,
  Label,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  Switch,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  toast,
} from "@/components/ui";
import { api } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";

// ── 类型定义 ─────────────────────────────────────────────────────

interface AuditEntry {
  id: number;
  node_id: string;
  username: string;
  source_ip: string;
  source_port: string;
  remote_ip: string;
  protocol: string;
  inbound_tag: string;
  destination: string;
  route_tag: string;
  created_at: string;
}

interface AuditLogsResponse {
  entries: AuditEntry[];
}

type RuleType = "domain_keyword" | "port" | "ip";

interface Rule {
  id: string;
  type: RuleType;
  value: string;
  enabled: boolean;
  created_at: string;
}

interface RulesResponse {
  rules: Rule[];
}

interface UsersResponse {
  users: Array<{ username: string }>;
}

interface NodesResponse {
  nodes: Array<{ id: string; name: string }>;
}

// ── 时间范围快捷选项 ──────────────────────────────────────────────

const TIME_RANGE_OPTIONS = [
  { label: "audit.last1h", hours: 1 },
  { label: "audit.last6h", hours: 6 },
  { label: "audit.last12h", hours: 12 },
  { label: "audit.last24h", hours: 24 },
] as const;

const RULE_TYPE_LABELS: Record<RuleType, string> = {
  domain_keyword: "audit.domainKeyword",
  port: "audit.portLabel",
  ip: "audit.ipLabel",
};

const RULE_TYPE_PLACEHOLDERS: Record<RuleType, string> = {
  domain_keyword: "audit.likeTorrent",
  port: "audit.likePort",
  ip: "audit.likeIP",
};

// ── 类型：用户分析 ────────────────────────────────────────────────

interface UserAnalysis {
  username: string;
  connections: number;
  distinct_ips: number;
  total_bytes: number;
  last_seen: string;
}

// ── 工具函数 ─────────────────────────────────────────────────────

function formatBytes(n: number): string {
  if (n >= 1024 ** 3) return (n / 1024 ** 3).toFixed(1) + " GB";
  if (n >= 1024 ** 2) return (n / 1024 ** 2).toFixed(1) + " MB";
  if (n >= 1024) return (n / 1024).toFixed(1) + " KB";
  return n + " B";
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

function buildQuery(params: Record<string, string>): string {
  const qs = Object.entries(params)
    .filter(([, v]) => v.trim() !== "")
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join("&");
  return qs ? `?${qs}` : "";
}

// ── 历史日志子组件 ────────────────────────────────────────────────

function AuditLogsTab() {
  const { t } = useTranslation();
  const handleAuthError = useAuthErrorHandler();

  const [selectedHours, setSelectedHours] = useState<number>(1);
  const [username, setUsername] = useState("");
  const [nodeId, setNodeId] = useState("");

  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(false);

  const [totalCount, setTotalCount] = useState<number | null>(null);

  const [userList, setUserList] = useState<string[] | null>(null);
  const [nodeList, setNodeList] = useState<Array<{ id: string; name: string }> | null>(null);

  useEffect(() => {
    api.get<{ users: string[] }>("/audit/users")
      .then((data) => setUserList(data.users ?? []))
      .catch(() => setUserList(null));

    api.get<NodesResponse>("/nodes")
      .then((data) => setNodeList(data.nodes ?? []))
      .catch(() => setNodeList(null));

    api.get<{ count: number }>("/audit/count")
      .then((data) => setTotalCount(data.count))
      .catch(() => setTotalCount(null));
  }, []);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const since = new Date(Date.now() - selectedHours * 3600 * 1000).toISOString();
      const until = new Date().toISOString();

      const params: Record<string, string> = { since, until };
      const effectiveUsername = username === "__all__" ? "" : username.trim();
      const effectiveNodeId = nodeId === "__all__" ? "" : nodeId.trim();
      if (effectiveUsername) params.username = effectiveUsername;
      if (effectiveNodeId) params.node_id = effectiveNodeId;

      const query = buildQuery(params);
      const data = await api.get<AuditLogsResponse>(`/audit/logs${query}`);
      const sorted = [...(data.entries ?? [])].sort(
        (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      );
      setEntries(sorted);
    } catch (err) {
      if (handleAuthError(err)) return;
      toast((err as Error).message || t("common.loadFailed"), "error");
    } finally {
      setLoading(false);
    }
  }, [selectedHours, username, nodeId, handleAuthError, t]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const copyAsText = useCallback(async () => {
    if (entries.length === 0) return;
    const lines = entries.map((e) =>
      [
        formatTime(e.created_at),
        e.username || "-",
        e.source_ip || "-",
        e.source_port || "-",
        e.protocol || "-",
        e.destination || "-",
        e.route_tag || "-",
      ].join("\t")
    );
    const text = [t("audit.copyHeader"), ...lines].join("\n");
    try {
      await copyText(text);
      toast(t("audit.copiedRecords", { count: entries.length }), "success");
    } catch {
      toast(t("audit.copyFailed"), "error");
    }
  }, [entries, t]);

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>
            {t("audit.title")}
            {totalCount !== null && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">
                {t("audit.totalInDB", { count: totalCount })}
              </span>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-4 items-end">
            <div className="space-y-1">
              <Label>{t("audit.timeRange")}</Label>
              <div className="flex gap-2">
                {TIME_RANGE_OPTIONS.map((opt) => (
                  <Button
                    key={opt.hours}
                    variant={selectedHours === opt.hours ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSelectedHours(opt.hours)}
                  >
                    {t(opt.label)}
                  </Button>
                ))}
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="audit-username">{t("audit.username")}</Label>
              {userList !== null ? (
                <Select value={username} onValueChange={setUsername}>
                  <SelectTrigger id="audit-username" className="w-40">
                    <SelectValue placeholder={t("common.all")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">{t("common.all")}</SelectItem>
                    {userList.map((u) => (
                      <SelectItem key={u} value={u}>
                        {u}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id="audit-username"
                  placeholder={t("audit.optional")}
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="w-40"
                  onKeyDown={(e) => e.key === "Enter" && fetchLogs()}
                />
              )}
            </div>

            <div className="space-y-1">
              <Label htmlFor="audit-node-id">{t("audit.node")}</Label>
              {nodeList !== null ? (
                <Select value={nodeId} onValueChange={setNodeId}>
                  <SelectTrigger id="audit-node-id" className="w-40">
                    <SelectValue placeholder={t("common.all")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all__">{t("common.all")}</SelectItem>
                    {nodeList.map((n) => (
                      <SelectItem key={n.id} value={n.id}>
                        {n.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id="audit-node-id"
                  placeholder={t("audit.optional")}
                  value={nodeId}
                  onChange={(e) => setNodeId(e.target.value)}
                  className="w-40"
                  onKeyDown={(e) => e.key === "Enter" && fetchLogs()}
                />
              )}
            </div>

            <Button onClick={fetchLogs} disabled={loading}>
              {loading ? t("audit.querying") : t("audit.query")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>
            {t("audit.queryResult")}
            {!loading && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">
                {t("audit.totalResults", { count: entries.length })}
              </span>
            )}
          </CardTitle>
          {entries.length > 0 && (
            <Button variant="outline" size="sm" onClick={copyAsText}>
              {t("audit.copyAsText")}
            </Button>
          )}
        </CardHeader>
        <CardContent className="p-0">
          {entries.length === 0 && !loading ? (
            <div className="flex items-center justify-center py-16 text-muted-foreground text-sm">
              {t("common.noData")}
            </div>
          ) : (
            <Table containerClassName="max-h-[600px]">
              <TableHeader className="sticky top-0 z-10 bg-[hsl(var(--card))]">
                <TableRow>
                  <TableHead>{t("audit.time")}</TableHead>
                  <TableHead>{t("audit.user")}</TableHead>
                  <TableHead>{t("audit.sourceIP")}</TableHead>
                  <TableHead>{t("audit.sourcePort")}</TableHead>
                  <TableHead>{t("common.protocol")}</TableHead>
                  <TableHead>{t("audit.targetAddress")}</TableHead>
                  <TableHead>{t("audit.routeOut")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell className="whitespace-nowrap text-sm">
                      {formatTime(entry.created_at)}
                    </TableCell>
                    <TableCell className="text-sm">{entry.username || "-"}</TableCell>
                    <TableCell className="font-mono text-sm">{entry.source_ip || "-"}</TableCell>
                    <TableCell className="font-mono text-sm">{entry.source_port || "-"}</TableCell>
                    <TableCell className="text-sm">{entry.protocol || "-"}</TableCell>
                    <TableCell className="font-mono text-sm min-w-64">{entry.destination || "-"}</TableCell>
                    <TableCell className="text-sm">{entry.route_tag || "-"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ── 告警规则子组件 ────────────────────────────────────────────────

function AlertRulesTab() {
  const { t } = useTranslation();

  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);

  const [newType, setNewType] = useState<RuleType>("domain_keyword");
  const [newValue, setNewValue] = useState("");
  const [adding, setAdding] = useState(false);

  const [togglingIds, setTogglingIds] = useState<Set<string>>(new Set());
  const [deletingIds, setDeletingIds] = useState<Set<string>>(new Set());

  const handleAuthError = useAuthErrorHandler();

  const fetchRules = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.get<RulesResponse>("/audit/rules");
      setRules(data.rules ?? []);
    } catch (err) {
      if (handleAuthError(err)) return;
      toast((err as Error).message || t("audit.loadRulesFailed"), "error");
    } finally {
      setLoading(false);
    }
  }, [handleAuthError, t]);

  useEffect(() => {
    fetchRules();
  }, [fetchRules]);

  const handleAdd = useCallback(async () => {
    const trimmed = newValue.trim();
    if (!trimmed) {
      toast(t("audit.ruleValueRequired"), "error");
      return;
    }
    setAdding(true);
    try {
      const created = await api.post<Rule>("/audit/rules", {
        type: newType,
        value: trimmed,
      });
      setRules((prev) => [created, ...prev]);
      setNewValue("");
      toast(t("audit.ruleAdded"), "success");
    } catch (err) {
      if (handleAuthError(err)) return;
      toast((err as Error).message || t("audit.addRuleFailed"), "error");
    } finally {
      setAdding(false);
    }
  }, [newType, newValue, handleAuthError, t]);

  const handleToggle = useCallback(
    async (rule: Rule) => {
      setTogglingIds((prev) => new Set(prev).add(rule.id));
      try {
        await api.post<Rule>(`/audit/rules/${rule.id}`, { enabled: !rule.enabled });
        setRules((prev) =>
          prev.map((r) => (r.id === rule.id ? { ...r, enabled: !r.enabled } : r))
        );
      } catch (err) {
        if (handleAuthError(err)) return;
        toast((err as Error).message || t("common.operationFailed"), "error");
      } finally {
        setTogglingIds((prev) => {
          const next = new Set(prev);
          next.delete(rule.id);
          return next;
        });
      }
    },
    [handleAuthError, t]
  );

  const handleDelete = useCallback(
    async (id: string) => {
      setDeletingIds((prev) => new Set(prev).add(id));
      try {
        await api.del(`/audit/rules/${id}`);
        setRules((prev) => prev.filter((r) => r.id !== id));
        toast(t("audit.ruleDeleted"), "success");
      } catch (err) {
        if (handleAuthError(err)) return;
        toast((err as Error).message || t("common.deleteFailed"), "error");
      } finally {
        setDeletingIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      }
    },
    [handleAuthError, t]
  );

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("audit.newRule")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-4 items-end">
            <div className="space-y-1">
              <Label htmlFor="rule-type">{t("common.type")}</Label>
              <Select
                value={newType}
                onValueChange={(v) => setNewType(v as RuleType)}
              >
                <SelectTrigger id="rule-type" className="w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="domain_keyword">{t("audit.domainKeyword")}</SelectItem>
                  <SelectItem value="port">{t("audit.portLabel")}</SelectItem>
                  <SelectItem value="ip">{t("audit.ipLabel")}</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1">
              <Label htmlFor="rule-value">{t("audit.value")}</Label>
              <Input
                id="rule-value"
                placeholder={t(RULE_TYPE_PLACEHOLDERS[newType])}
                value={newValue}
                onChange={(e) => setNewValue(e.target.value)}
                className="w-52"
                onKeyDown={(e) => e.key === "Enter" && handleAdd()}
              />
            </div>

            <Button onClick={handleAdd} disabled={adding}>
              {adding ? t("audit.adding") : t("common.add")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            {t("audit.ruleList")}
            {!loading && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">
                {t("audit.totalRules", { count: rules.length })}
              </span>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {rules.length === 0 && !loading ? (
            <div className="flex items-center justify-center py-16 text-muted-foreground text-sm">
              {t("audit.noRules")}
            </div>
          ) : (
            <Table containerClassName="max-h-[600px]">
              <TableHeader className="sticky top-0 z-10 bg-[hsl(var(--card))]">
                <TableRow>
                  <TableHead>{t("common.type")}</TableHead>
                  <TableHead>{t("audit.value")}</TableHead>
                  <TableHead>{t("audit.createTime")}</TableHead>
                  <TableHead>{t("common.status")}</TableHead>
                  <TableHead className="text-right">{t("common.action")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell className="text-sm">
                      {t(RULE_TYPE_LABELS[rule.type] ?? rule.type)}
                    </TableCell>
                    <TableCell className="font-mono text-sm">{rule.value}</TableCell>
                    <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                      {formatTime(rule.created_at)}
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={rule.enabled}
                        disabled={togglingIds.has(rule.id)}
                        onCheckedChange={() => handleToggle(rule)}
                        aria-label={rule.enabled ? t("audit.disableRule") : t("audit.enableRule")}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={deletingIds.has(rule.id)}
                        onClick={() => handleDelete(rule.id)}
                      >
                        {deletingIds.has(rule.id) ? t("common.deleting") : t("common.delete")}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ── 用户分析子组件 ────────────────────────────────────────────────

function AnalysisTab() {
  const { t } = useTranslation();
  const handleAuthError = useAuthErrorHandler();
  const [selectedHours, setSelectedHours] = useState<number>(24);
  const [users, setUsers] = useState<UserAnalysis[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchAnalysis = useCallback(async () => {
    setLoading(true);
    try {
      const since = new Date(Date.now() - selectedHours * 3600 * 1000).toISOString();
      const until = new Date().toISOString();
      const data = await api.get<{ users: UserAnalysis[] }>(
        `/audit/analysis?since=${encodeURIComponent(since)}&until=${encodeURIComponent(until)}`
      );
      setUsers(data.users ?? []);
    } catch (err) {
      if (handleAuthError(err)) return;
      toast((err as Error).message || t("common.loadFailed"), "error");
    } finally {
      setLoading(false);
    }
  }, [selectedHours, handleAuthError, t]);

  useEffect(() => { fetchAnalysis(); }, [fetchAnalysis]);

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader><CardTitle>{t("audit.userAnalysis")}</CardTitle></CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-4 items-end">
            <div className="space-y-1">
              <Label>{t("audit.timeRange")}</Label>
              <div className="flex gap-2">
                {TIME_RANGE_OPTIONS.map((opt) => (
                  <Button
                    key={opt.hours}
                    variant={selectedHours === opt.hours ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSelectedHours(opt.hours)}
                  >
                    {t(opt.label)}
                  </Button>
                ))}
              </div>
            </div>
            <Button onClick={fetchAnalysis} disabled={loading}>
              {loading ? t("audit.analyzing") : t("audit.analyze")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            {t("audit.userSummary")}
            {!loading && (
              <span className="ml-2 text-sm font-normal text-muted-foreground">
                {t("audit.totalUsers", { count: users.length })}
              </span>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <p className="px-4 pt-3 pb-1 text-xs text-muted-foreground">
            {t("audit.ipHint")}
          </p>
          {users.length === 0 && !loading ? (
            <div className="flex items-center justify-center py-16 text-muted-foreground text-sm">
              {t("common.noData")}
            </div>
          ) : (
            <Table containerClassName="max-h-[600px]">
                <TableHeader className="sticky top-0 z-10 bg-[hsl(var(--card))]">
                  <TableRow>
                    <TableHead>{t("audit.userCol")}</TableHead>
                    <TableHead>{t("audit.uniqueIPs")}</TableHead>
                    <TableHead>{t("audit.connections")}</TableHead>
                    <TableHead>{t("audit.periodTraffic")}</TableHead>
                    <TableHead>{t("audit.lastActive")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((u) => (
                    <TableRow key={u.username}>
                      <TableCell className="text-sm font-medium">{u.username}</TableCell>
                      <TableCell className={`text-sm font-mono font-semibold ${u.distinct_ips > 3 ? "text-orange-500" : ""}`}>
                        {u.distinct_ips}
                      </TableCell>
                      <TableCell className="text-sm font-mono">{u.connections}</TableCell>
                      <TableCell className="text-sm font-mono">
                        {u.total_bytes > 0 ? formatBytes(u.total_bytes) : "-"}
                      </TableCell>
                      <TableCell className="text-sm whitespace-nowrap">
                        {formatTime(u.last_seen)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ── 页面组件 ─────────────────────────────────────────────────────

export default function AuditPage() {
  const { t } = useTranslation();
  return (
    <div className="p-6 space-y-4">
      <Tabs defaultValue="logs">
        <TabsList>
          <TabsTrigger value="logs">{t("audit.historyLogs")}</TabsTrigger>
          <TabsTrigger value="rules">{t("audit.alertRules")}</TabsTrigger>
          <TabsTrigger value="analysis">{t("audit.userAnalysisTab")}</TabsTrigger>
        </TabsList>

        <TabsContent value="logs">
          <AuditLogsTab />
        </TabsContent>

        <TabsContent value="rules">
          <AlertRulesTab />
        </TabsContent>

        <TabsContent value="analysis">
          <AnalysisTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
