import { useEffect, useState, useCallback, type FormEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  Card,
  CardContent,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  Badge,
  Button,
  Input,
  Label,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Switch,
  Checkbox,
  ConfirmDialog,
  toast,
} from "@/components/ui";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cfApi, nodeDomainApi, api, ixApi, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";
import type {
  CFDomain,
  CFZone,
  CFDNSRecord,
  CreateCFDNSRecordRequest,
  NodeDomain,
  Node,
  NodesResponse,
  IXDomain,
  IXDomainsResponse,
} from "@/lib/types";

// ── Icons ────────────────────────────────────────────────────────

function IconPlus({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  );
}

function IconTrash({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </svg>
  );
}

function IconEdit({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
      <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
    </svg>
  );
}

function IconChevronDown({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <polyline points="6 9 12 15 18 9" />
    </svg>
  );
}

function IconChevronRight({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <polyline points="9 18 15 12 9 6" />
    </svg>
  );
}

function IconLoader({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <line x1="12" y1="2" x2="12" y2="6" />
      <line x1="12" y1="18" x2="12" y2="22" />
      <line x1="4.93" y1="4.93" x2="7.76" y2="7.76" />
      <line x1="16.24" y1="16.24" x2="19.07" y2="19.07" />
      <line x1="2" y1="12" x2="6" y2="12" />
      <line x1="18" y1="12" x2="22" y2="12" />
      <line x1="4.93" y1="19.07" x2="7.76" y2="16.24" />
      <line x1="16.24" y1="7.76" x2="19.07" y2="4.93" />
    </svg>
  );
}

/** CF 代理云朵图标 */
function IconCloud({ proxied, className }: { proxied: boolean; className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill={proxied ? "#f38020" : "none"} stroke={proxied ? "#f38020" : "currentColor"} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" />
    </svg>
  );
}

// ── Constants ────────────────────────────────────────────────────

const DNS_RECORD_TYPES = ["A", "AAAA", "CNAME"] as const;

const ZONE_STATUS_VARIANT: Record<string, "default" | "secondary" | "outline"> = {
  active: "default",
  pending: "secondary",
};

// ── Skeleton ────────────────────────────────────────────────────

function SkeletonCard() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex items-center gap-4">
          <div className="h-5 w-40 animate-pulse rounded bg-[hsl(var(--muted))]" />
          <div className="h-4 w-20 animate-pulse rounded bg-[hsl(var(--muted))]" />
          <div className="ml-auto h-8 w-16 animate-pulse rounded bg-[hsl(var(--muted))]" />
        </div>
      </CardContent>
    </Card>
  );
}

// ── 添加域名 Dialog ─────────────────────────────────────────────

function AddDomainDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}) {
  const navigate = useNavigate();
  const [step, setStep] = useState<"token" | "select">("token");
  const [cfToken, setCfToken] = useState("");
  const [remark, setRemark] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [zones, setZones] = useState<CFZone[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function reset() {
    setStep("token");
    setCfToken("");
    setRemark("");
    setVerifying(false);
    setZones([]);
    setSelected(new Set());
    setSubmitting(false);
    setError(null);
  }

  async function handleVerify(e: FormEvent) {
    e.preventDefault();
    if (!cfToken.trim()) return;
    setVerifying(true);
    setError(null);
    try {
      const res = await cfApi.verifyToken(cfToken.trim());
      setZones(res.zones ?? []);
      if ((res.zones ?? []).length === 0) {
        setError("该 Token 下没有可用的域名");
      } else {
        setStep("select");
      }
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      setError(err instanceof Error ? err.message : "验证失败");
    } finally {
      setVerifying(false);
    }
  }

  function toggleZone(zoneId: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(zoneId)) {
        next.delete(zoneId);
      } else {
        next.add(zoneId);
      }
      return next;
    });
  }

  async function handleSubmit() {
    if (selected.size === 0) return;
    setSubmitting(true);
    setError(null);
    try {
      for (const zoneId of selected) {
        const zone = zones.find((z) => z.id === zoneId);
        if (!zone) continue;
        await cfApi.createDomain({
          cf_token: cfToken.trim(),
          zone_id: zone.id,
          zone_name: zone.name,
          remark: remark.trim(),
        });
      }
      toast.success(`成功添加 ${selected.size} 个域名`);
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      setError(err instanceof Error ? err.message : "添加失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v);
        if (!v) reset();
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>添加域名</DialogTitle>
          <DialogDescription>
            {step === "token"
              ? "输入 Cloudflare API Token，验证后选择要管理的域名。"
              : "选择要添加的域名，可多选。"}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-4">
          {error && (
            <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
              {error}
            </div>
          )}

          {step === "token" && (
            <form onSubmit={handleVerify} className="grid gap-4">
              <div className="space-y-2">
                <Label htmlFor="cf-token">CF API Token *</Label>
                <Input
                  id="cf-token"
                  type="password"
                  required
                  value={cfToken}
                  onChange={(e) => setCfToken(e.target.value)}
                  placeholder="输入 Cloudflare API Token"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cf-remark">备注</Label>
                <Input
                  id="cf-remark"
                  value={remark}
                  onChange={(e) => setRemark(e.target.value)}
                  placeholder="可选，如「主账号」"
                />
              </div>
              <Button type="submit" disabled={verifying || !cfToken.trim()}>
                {verifying && <IconLoader className="mr-2 h-4 w-4 animate-spin" />}
                {verifying ? "验证中…" : "验证 Token"}
              </Button>
            </form>
          )}

          {step === "select" && (
            <>
              <ScrollArea className="max-h-72 rounded-md border border-[hsl(var(--border))]">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10 px-4" />
                      <TableHead className="px-4">域名</TableHead>
                      <TableHead className="px-4">状态</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {zones.map((zone) => (
                      <TableRow
                        key={zone.id}
                        className="cursor-pointer"
                        onClick={() => toggleZone(zone.id)}
                      >
                        <TableCell className="px-4">
                          <Checkbox
                            checked={selected.has(zone.id)}
                            onCheckedChange={() => toggleZone(zone.id)}
                          />
                        </TableCell>
                        <TableCell className="px-4 font-medium text-[hsl(var(--foreground))]">
                          {zone.name}
                        </TableCell>
                        <TableCell className="px-4">
                          <Badge variant={ZONE_STATUS_VARIANT[zone.status] ?? "outline"}>
                            {zone.status}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </ScrollArea>

              <div className="space-y-2">
                <Label htmlFor="cf-remark-2">备注</Label>
                <Input
                  id="cf-remark-2"
                  value={remark}
                  onChange={(e) => setRemark(e.target.value)}
                  placeholder="可选"
                />
              </div>
            </>
          )}
        </div>

        <DialogFooter>
          {step === "select" && (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setStep("token");
                setSelected(new Set());
              }}
            >
              上一步
            </Button>
          )}
          <DialogClose asChild>
            <Button type="button" variant="outline">
              取消
            </Button>
          </DialogClose>
          {step === "select" && (
            <Button
              type="button"
              disabled={submitting || selected.size === 0}
              onClick={handleSubmit}
            >
              {submitting ? "添加中…" : `添加 (${selected.size})`}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── DNS 记录编辑 Dialog ─────────────────────────────────────────

interface RecordFormState {
  type: string;
  name: string;
  content: string;
  ttl: string;
  proxied: boolean;
}

const EMPTY_RECORD_FORM: RecordFormState = {
  type: "A",
  name: "",
  content: "",
  ttl: "1",
  proxied: false,
};

function RecordFormDialog({
  open,
  onOpenChange,
  domainId,
  zoneName,
  editingRecord,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  domainId: string;
  zoneName: string;
  editingRecord: CFDNSRecord | null;
  onSuccess: () => void;
}) {
  const navigate = useNavigate();
  const isEdit = editingRecord !== null;
  const [form, setForm] = useState<RecordFormState>(EMPTY_RECORD_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 打开时初始化表单
  useEffect(() => {
    if (open && editingRecord) {
      setForm({
        type: editingRecord.type,
        name: editingRecord.name,
        content: editingRecord.content,
        ttl: String(editingRecord.ttl),
        proxied: editingRecord.proxied,
      });
    } else if (open) {
      setForm(EMPTY_RECORD_FORM);
    }
    setError(null);
  }, [open, editingRecord]);

  function patchForm(patch: Partial<RecordFormState>) {
    setForm((prev) => ({ ...prev, ...patch }));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);

    const body: CreateCFDNSRecordRequest = {
      type: form.type,
      name: form.name.trim(),
      content: form.content.trim(),
      ttl: parseInt(form.ttl, 10) || 1,
      proxied: form.proxied,
    };

    try {
      if (isEdit && editingRecord) {
        await cfApi.updateRecord(domainId, editingRecord.id, body);
        toast.success("DNS 记录已更新");
      } else {
        await cfApi.createRecord(domainId, body);
        toast.success("DNS 记录已添加");
      }
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      setError(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{isEdit ? "编辑 DNS 记录" : "添加 DNS 记录"}</DialogTitle>
            <DialogDescription>
              {zoneName}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            {error && (
              <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
                {error}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="rec-type">类型</Label>
              <Select
                value={form.type}
                onValueChange={(v) => patchForm({ type: v })}
              >
                <SelectTrigger id="rec-type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {DNS_RECORD_TYPES.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="rec-name">名称 *</Label>
              <Input
                id="rec-name"
                required
                value={form.name}
                onChange={(e) => patchForm({ name: e.target.value })}
                placeholder={`sub.${zoneName} 或 @ 表示根域名`}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="rec-content">
                {form.type === "CNAME" ? "目标 *" : "IP 地址 *"}
              </Label>
              <Input
                id="rec-content"
                required
                value={form.content}
                onChange={(e) => patchForm({ content: e.target.value })}
                placeholder={
                  form.type === "A"
                    ? "1.2.3.4"
                    : form.type === "AAAA"
                      ? "2001:db8::1"
                      : "target.example.com"
                }
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="rec-ttl">TTL</Label>
              <Select
                value={form.ttl}
                onValueChange={(v) => patchForm({ ttl: v })}
              >
                <SelectTrigger id="rec-ttl">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1">Auto</SelectItem>
                  <SelectItem value="60">1 分钟</SelectItem>
                  <SelectItem value="300">5 分钟</SelectItem>
                  <SelectItem value="600">10 分钟</SelectItem>
                  <SelectItem value="3600">1 小时</SelectItem>
                  <SelectItem value="86400">1 天</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center gap-3">
              <Switch
                id="rec-proxied"
                checked={form.proxied}
                onCheckedChange={(checked) => patchForm({ proxied: !!checked })}
              />
              <Label htmlFor="rec-proxied" className="flex items-center gap-2">
                <IconCloud proxied={form.proxied} className="h-5 w-5" />
                Cloudflare 代理
              </Label>
            </div>
          </div>

          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                取消
              </Button>
            </DialogClose>
            <Button type="submit" disabled={submitting}>
              {submitting ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// ── DNS 记录面板 ────────────────────────────────────────────────

function DnsRecordsPanel({ domain }: { domain: CFDomain }) {
  const navigate = useNavigate();
  const [records, setRecords] = useState<CFDNSRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filterType, setFilterType] = useState<string>("all");

  // 编辑/添加 dialog
  const [recordFormOpen, setRecordFormOpen] = useState(false);
  const [editingRecord, setEditingRecord] = useState<CFDNSRecord | null>(null);

  // 删除 confirm
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletingRecord, setDeletingRecord] = useState<CFDNSRecord | null>(null);
  const [deleting, setDeleting] = useState(false);

  const fetchRecords = useCallback(() => {
    setLoading(true);
    setError(null);
    const typeParam = filterType === "all" ? undefined : filterType;
    cfApi
      .listRecords(domain.id, typeParam)
      .then((res) => setRecords(res.records ?? []))
      .catch((err) => {
        if (err instanceof AuthError) {
          clearToken();
          navigate({ to: "/panel/login" });
          return;
        }
        setError(err instanceof Error ? err.message : "加载失败");
      })
      .finally(() => setLoading(false));
  }, [domain.id, filterType, navigate]);

  useEffect(() => {
    fetchRecords();
  }, [fetchRecords]);

  async function handleDeleteRecord() {
    if (!deletingRecord) return;
    setDeleting(true);
    try {
      await cfApi.deleteRecord(domain.id, deletingRecord.id);
      toast.success("DNS 记录已删除");
      setDeleteOpen(false);
      setDeletingRecord(null);
      fetchRecords();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      toast.error(err instanceof Error ? err.message : "删除失败");
    } finally {
      setDeleting(false);
    }
  }

  function formatTTL(ttl: number): string {
    if (ttl === 1) return "Auto";
    if (ttl < 60) return `${ttl}s`;
    if (ttl < 3600) return `${Math.floor(ttl / 60)}m`;
    if (ttl < 86400) return `${Math.floor(ttl / 3600)}h`;
    return `${Math.floor(ttl / 86400)}d`;
  }

  return (
    <div className="space-y-4">
      {/* 工具栏 */}
      <div className="flex items-center gap-3">
        <Select
          value={filterType}
          onValueChange={(v) => setFilterType(v)}
        >
          <SelectTrigger className="w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部</SelectItem>
            {DNS_RECORD_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button
          size="sm"
          onClick={() => {
            setEditingRecord(null);
            setRecordFormOpen(true);
          }}
        >
          <IconPlus className="mr-1 h-3.5 w-3.5" />
          添加记录
        </Button>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
          {error}
          <Button variant="ghost" size="sm" className="ml-2" onClick={fetchRecords}>
            重试
          </Button>
        </div>
      )}

      {/* 记录表格 */}
      <div className="overflow-x-auto rounded-md border border-[hsl(var(--border))]">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-20 px-4">类型</TableHead>
              <TableHead className="px-4">名称</TableHead>
              <TableHead className="px-4">内容</TableHead>
              <TableHead className="hidden w-20 px-4 sm:table-cell">TTL</TableHead>
              <TableHead className="hidden w-16 px-4 sm:table-cell">代理</TableHead>
              <TableHead className="w-24 px-4">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 6 }).map((_, j) => (
                    <TableCell key={j} className="px-4 py-3">
                      <div className="h-4 w-full max-w-[80px] animate-pulse rounded bg-[hsl(var(--muted))]" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : records.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="h-24 text-center text-[hsl(var(--muted-foreground))]"
                >
                  暂无 DNS 记录
                </TableCell>
              </TableRow>
            ) : (
              records.map((rec) => (
                <TableRow key={rec.id}>
                  <TableCell className="px-4">
                    <Badge variant="outline">{rec.type}</Badge>
                  </TableCell>
                  <TableCell className="px-4 font-mono text-sm text-[hsl(var(--foreground))]">
                    {rec.name}
                  </TableCell>
                  <TableCell className="px-4 font-mono text-sm text-[hsl(var(--muted-foreground))]">
                    {rec.content}
                  </TableCell>
                  <TableCell className="hidden px-4 text-sm text-[hsl(var(--muted-foreground))] sm:table-cell">
                    {formatTTL(rec.ttl)}
                  </TableCell>
                  <TableCell className="hidden px-4 sm:table-cell">
                    <IconCloud proxied={rec.proxied} className="h-5 w-5" />
                  </TableCell>
                  <TableCell className="px-4">
                    <div className="flex gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setEditingRecord(rec);
                          setRecordFormOpen(true);
                        }}
                      >
                        <IconEdit className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-[hsl(var(--destructive))] hover:text-[hsl(var(--destructive))]"
                        onClick={() => {
                          setDeletingRecord(rec);
                          setDeleteOpen(true);
                        }}
                      >
                        <IconTrash className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* 编辑/添加记录 Dialog */}
      <RecordFormDialog
        open={recordFormOpen}
        onOpenChange={setRecordFormOpen}
        domainId={domain.id}
        zoneName={domain.zone_name}
        editingRecord={editingRecord}
        onSuccess={fetchRecords}
      />

      {/* 删除记录确认 */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="删除 DNS 记录"
        description={
          <>
            确定要删除{" "}
            <span className="font-semibold text-[hsl(var(--foreground))]">
              {deletingRecord?.type} {deletingRecord?.name}
            </span>{" "}
            吗？此操作不可撤销。
          </>
        }
        confirmLabel="删除"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDeleteRecord}
      />
    </div>
  );
}

// ── 节点映射面板 ────────────────────────────────────────────────

function NodeMappingPanel({ domain, nodes }: { domain: CFDomain; nodes: Node[] }) {
  const navigate = useNavigate();
  const [records, setRecords] = useState<NodeDomain[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);

  const fetchRecords = useCallback(() => {
    setLoading(true);
    nodeDomainApi
      .list(domain.id)
      .then((res) => setRecords(res.node_domains ?? []))
      .catch((err) => {
        if (err instanceof AuthError) {
          clearToken();
          navigate({ to: "/panel/login" });
          return;
        }
        toast.error(err instanceof Error ? err.message : "加载失败");
      })
      .finally(() => setLoading(false));
  }, [domain.id, navigate]);

  useEffect(() => {
    fetchRecords();
  }, [fetchRecords]);

  async function handleSync() {
    setSyncing(true);
    try {
      const res = await nodeDomainApi.sync({ cf_domain_id: domain.id });
      toast.success(`同步完成，共 ${res.synced} 条记录`);
      fetchRecords();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      toast.error(err instanceof Error ? err.message : "同步失败");
    } finally {
      setSyncing(false);
    }
  }

  async function handleUpdateNode(id: string, nodeId: string) {
    try {
      const updated = await nodeDomainApi.updateNodeID(id, nodeId);
      setRecords((prev) => prev.map((r) => (r.id === id ? updated : r)));
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      toast.error(err instanceof Error ? err.message : "更新失败");
    }
  }

  async function handleDelete(id: string) {
    try {
      await nodeDomainApi.delete(id);
      setRecords((prev) => prev.filter((r) => r.id !== id));
      toast.success("已删除");
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      toast.error(err instanceof Error ? err.message : "删除失败");
    }
  }

  const nodeMap = Object.fromEntries(nodes.map((n) => [n.id, n.name]));

  return (
    <div className="space-y-4">
      {/* 工具栏 */}
      <div className="flex items-center gap-3">
        <Button size="sm" onClick={handleSync} disabled={syncing}>
          {syncing ? (
            <IconLoader className="mr-1 h-3.5 w-3.5 animate-spin" />
          ) : (
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="mr-1 h-3.5 w-3.5">
              <polyline points="23 4 23 10 17 10" />
              <polyline points="1 20 1 14 7 14" />
              <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
            </svg>
          )}
          从 CF 同步
        </Button>
        <span className="text-sm text-[hsl(var(--muted-foreground))]">
          共 {records.length} 条，按 IP 自动匹配节点
        </span>
      </div>

      {/* 记录表格 */}
      <div className="overflow-x-auto rounded-md border border-[hsl(var(--border))]">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-20 px-4">类型</TableHead>
              <TableHead className="px-4">域名</TableHead>
              <TableHead className="px-4">内容（IP/目标）</TableHead>
              <TableHead className="hidden w-16 px-4 sm:table-cell">代理</TableHead>
              <TableHead className="px-4">节点</TableHead>
              <TableHead className="w-16 px-4">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 6 }).map((_, j) => (
                    <TableCell key={j} className="px-4 py-3">
                      <div className="h-4 w-full max-w-[80px] animate-pulse rounded bg-[hsl(var(--muted))]" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : records.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-[hsl(var(--muted-foreground))]">
                  暂无记录，点击"从 CF 同步"导入
                </TableCell>
              </TableRow>
            ) : (
              records.map((rec) => (
                <TableRow key={rec.id}>
                  <TableCell className="px-4">
                    <Badge variant="outline">{rec.record_type}</Badge>
                  </TableCell>
                  <TableCell className="px-4 font-mono text-sm text-[hsl(var(--foreground))]">
                    {rec.fqdn}
                  </TableCell>
                  <TableCell className="px-4 font-mono text-sm text-[hsl(var(--muted-foreground))]">
                    {rec.content}
                  </TableCell>
                  <TableCell className="hidden px-4 sm:table-cell">
                    <IconCloud proxied={rec.proxied} className="h-5 w-5" />
                  </TableCell>
                  <TableCell className="px-4">
                    <Select
                      value={rec.node_id || "__unassigned__"}
                      onValueChange={(v) => handleUpdateNode(rec.id, v === "__unassigned__" ? "" : v)}
                    >
                      <SelectTrigger className="h-7 w-40 text-sm">
                        <SelectValue>
                          {rec.node_id ? (nodeMap[rec.node_id] ?? rec.node_id) : (
                            <span className="text-[hsl(var(--muted-foreground))]">未分配</span>
                          )}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="__unassigned__">
                          <span className="text-[hsl(var(--muted-foreground))]">未分配</span>
                        </SelectItem>
                        {nodes.map((n) => (
                          <SelectItem key={n.id} value={n.id}>
                            {n.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell className="px-4">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-[hsl(var(--destructive))] hover:text-[hsl(var(--destructive))]"
                      onClick={() => handleDelete(rec.id)}
                    >
                      <IconTrash className="h-3.5 w-3.5" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

// ── 域名卡片 ────────────────────────────────────────────────────

function DomainCard({
  domain,
  nodes,
  onDelete,
}: {
  domain: CFDomain;
  nodes: Node[];
  onDelete: (domain: CFDomain) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [activeTab, setActiveTab] = useState<"dns" | "nodes">("dns");

  return (
    <Card>
      <CardContent className="p-0">
        {/* 域名信息行 */}
        <div
          className="flex cursor-pointer items-center gap-4 px-6 py-4 transition-colors hover:bg-[hsl(var(--accent))]/50"
          onClick={() => setExpanded((prev) => !prev)}
        >
          {/* 展开箭头 */}
          <div className="shrink-0">
            {expanded ? (
              <IconChevronDown className="h-4 w-4 text-[hsl(var(--muted-foreground))]" />
            ) : (
              <IconChevronRight className="h-4 w-4 text-[hsl(var(--muted-foreground))]" />
            )}
          </div>

          {/* 域名 */}
          <span className="font-semibold text-[hsl(var(--foreground))]">
            {domain.zone_name}
          </span>

          {/* 备注 */}
          {domain.remark && (
            <Badge variant="secondary" className="text-xs">
              {domain.remark}
            </Badge>
          )}

          {/* Token 脱敏 */}
          <span className="font-mono text-xs text-[hsl(var(--muted-foreground))]">
            {domain.cf_token}
          </span>

          {/* 操作 */}
          <div className="ml-auto flex gap-2" onClick={(e) => e.stopPropagation()}>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => onDelete(domain)}
            >
              <IconTrash className="mr-1 h-3.5 w-3.5" />
              删除
            </Button>
          </div>
        </div>

        {/* 展开区域 */}
        {expanded && (
          <div className="border-t border-[hsl(var(--border))]">
            {/* Tab 切换 */}
            <div className="flex border-b border-[hsl(var(--border))] px-6">
              <button
                type="button"
                className={`-mb-px border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                  activeTab === "dns"
                    ? "border-[hsl(var(--primary))] text-[hsl(var(--primary))]"
                    : "border-transparent text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                }`}
                onClick={() => setActiveTab("dns")}
              >
                DNS 记录
              </button>
              <button
                type="button"
                className={`-mb-px border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
                  activeTab === "nodes"
                    ? "border-[hsl(var(--primary))] text-[hsl(var(--primary))]"
                    : "border-transparent text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                }`}
                onClick={() => setActiveTab("nodes")}
              >
                节点映射
              </button>
            </div>

            <div className="px-6 py-4">
              {activeTab === "dns" ? (
                <DnsRecordsPanel domain={domain} />
              ) : (
                <NodeMappingPanel domain={domain} nodes={nodes} />
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ── IX 中转域名表单 Dialog ───────────────────────────────────────

interface IXDomainFormState {
  name: string;
  domain: string;
  remark: string;
}

const EMPTY_IX_FORM: IXDomainFormState = { name: "", domain: "", remark: "" };

function IXDomainFormDialog({
  open,
  onOpenChange,
  editing,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editing: IXDomain | null;
  onSuccess: () => void;
}) {
  const navigate = useNavigate();
  const isEdit = editing !== null;
  const [form, setForm] = useState<IXDomainFormState>(EMPTY_IX_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open && editing) {
      setForm({ name: editing.name, domain: editing.domain, remark: editing.remark });
    } else if (open) {
      setForm(EMPTY_IX_FORM);
    }
    setError(null);
  }, [open, editing]);

  function patch(p: Partial<IXDomainFormState>) {
    setForm((prev) => ({ ...prev, ...p }));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    const body = { name: form.name.trim(), domain: form.domain.trim(), remark: form.remark.trim() };
    try {
      if (isEdit && editing) {
        await ixApi.update(editing.id, body);
        toast.success("IX 域名已更新");
      } else {
        await ixApi.create(body);
        toast.success("IX 域名已添加");
      }
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      setError(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{isEdit ? "编辑 IX 域名" : "添加 IX 中转域名"}</DialogTitle>
            <DialogDescription>
              {isEdit ? "修改 IX 中转域名信息。" : "新增一条 IX 中转域名记录。"}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            {error && (
              <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
                {error}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="ix-name">名称 *</Label>
              <Input
                id="ix-name"
                required
                value={form.name}
                onChange={(e) => patch({ name: e.target.value })}
                placeholder="如「华东 IX」"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="ix-domain">域名 *</Label>
              <Input
                id="ix-domain"
                required
                value={form.domain}
                onChange={(e) => patch({ domain: e.target.value })}
                placeholder="如 relay.example.cn"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="ix-remark">备注</Label>
              <Input
                id="ix-remark"
                value={form.remark}
                onChange={(e) => patch({ remark: e.target.value })}
                placeholder="可选"
              />
            </div>
          </div>

          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                取消
              </Button>
            </DialogClose>
            <Button type="submit" disabled={submitting}>
              {submitting ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// ── 主页面 ──────────────────────────────────────────────────────

export default function DomainsPage() {
  const navigate = useNavigate();

  // CF 域名状态
  const [domains, setDomains] = useState<CFDomain[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // CF Dialog 状态
  const [addOpen, setAddOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletingDomain, setDeletingDomain] = useState<CFDomain | null>(null);
  const [deleting, setDeleting] = useState(false);

  // IX 域名状态
  const [ixDomains, setIxDomains] = useState<IXDomain[]>([]);
  const [ixLoading, setIxLoading] = useState(true);

  // IX Dialog 状态
  const [ixFormOpen, setIxFormOpen] = useState(false);
  const [ixEditing, setIxEditing] = useState<IXDomain | null>(null);
  const [ixDeleteOpen, setIxDeleteOpen] = useState(false);
  const [ixDeleting, setIxDeleting] = useState<IXDomain | null>(null);
  const [ixDeleteLoading, setIxDeleteLoading] = useState(false);

  const fetchDomains = useCallback(() => {
    setLoading(true);
    setIxLoading(true);
    setError(null);
    Promise.all([
      cfApi.listDomains(),
      api.get<NodesResponse>("/nodes"),
      ixApi.list(),
    ])
      .then(([domainsRes, nodesRes, ixRes]) => {
        setDomains(domainsRes.domains ?? []);
        setNodes(nodesRes.nodes ?? []);
        setIxDomains(ixRes.ix_domains ?? []);
      })
      .catch((err) => {
        if (err instanceof AuthError) {
          clearToken();
          navigate({ to: "/panel/login" });
          return;
        }
        setError(err instanceof Error ? err.message : "加载失败");
      })
      .finally(() => {
        setLoading(false);
        setIxLoading(false);
      });
  }, [navigate]);

  useEffect(() => {
    fetchDomains();
  }, [fetchDomains]);

  async function handleDelete() {
    if (!deletingDomain) return;
    setDeleting(true);
    try {
      await cfApi.deleteDomain(deletingDomain.id);
      toast.success(`已删除域名 ${deletingDomain.zone_name}`);
      setDeleteOpen(false);
      setDeletingDomain(null);
      fetchDomains();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      toast.error(err instanceof Error ? err.message : "删除失败");
    } finally {
      setDeleting(false);
    }
  }

  async function handleIxDelete() {
    if (!ixDeleting) return;
    setIxDeleteLoading(true);
    try {
      await ixApi.del(ixDeleting.id);
      toast.success(`已删除 IX 域名 ${ixDeleting.name}`);
      setIxDeleteOpen(false);
      setIxDeleting(null);
      fetchDomains();
    } catch (err) {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return;
      }
      toast.error(err instanceof Error ? err.message : "删除失败");
    } finally {
      setIxDeleteLoading(false);
    }
  }

  // 加载失败全屏错误
  if (error && !domains.length) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <Card className="w-full max-w-md">
          <CardContent className="pt-6 text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-[hsl(var(--destructive))]/10 text-[hsl(var(--destructive))]">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
                strokeLinecap="round"
                strokeLinejoin="round"
                className="h-6 w-6"
              >
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
            </div>
            <p className="mb-1 font-semibold text-[hsl(var(--foreground))]">
              加载失败
            </p>
            <p className="mb-4 text-sm text-[hsl(var(--muted-foreground))]">
              {error}
            </p>
            <Button variant="outline" onClick={fetchDomains}>
              重试
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="p-4 sm:p-6 lg:p-8">
      {/* 标题栏 */}
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold text-[hsl(var(--foreground))]">
          域名管理
        </h1>
        <Button onClick={() => setAddOpen(true)}>
          <IconPlus className="mr-1 h-4 w-4" />
          添加域名
        </Button>
      </div>

      {/* IX 中转域名区块 */}
      <div className="mb-8 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-[hsl(var(--foreground))]">
            IX 中转域名
          </h2>
          <Button
            size="sm"
            onClick={() => {
              setIxEditing(null);
              setIxFormOpen(true);
            }}
          >
            <IconPlus className="mr-1 h-3.5 w-3.5" />
            添加 IX 域名
          </Button>
        </div>

        <div className="overflow-x-auto rounded-md border border-[hsl(var(--border))]">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="px-4">名称</TableHead>
                <TableHead className="px-4">域名</TableHead>
                <TableHead className="px-4">备注</TableHead>
                <TableHead className="w-24 px-4">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {ixLoading ? (
                Array.from({ length: 2 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 4 }).map((_, j) => (
                      <TableCell key={j} className="px-4 py-3">
                        <div className="h-4 w-full max-w-[120px] animate-pulse rounded bg-[hsl(var(--muted))]" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : ixDomains.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    className="h-24 text-center text-[hsl(var(--muted-foreground))]"
                  >
                    暂无 IX 中转域名，点击右上角添加
                  </TableCell>
                </TableRow>
              ) : (
                ixDomains.map((d) => (
                  <TableRow key={d.id}>
                    <TableCell className="px-4 font-medium text-[hsl(var(--foreground))]">
                      {d.name}
                    </TableCell>
                    <TableCell className="px-4 font-mono text-sm text-[hsl(var(--muted-foreground))]">
                      {d.domain}
                    </TableCell>
                    <TableCell className="px-4 text-sm text-[hsl(var(--muted-foreground))]">
                      {d.remark || "—"}
                    </TableCell>
                    <TableCell className="px-4">
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setIxEditing(d);
                            setIxFormOpen(true);
                          }}
                        >
                          <IconEdit className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-[hsl(var(--destructive))] hover:text-[hsl(var(--destructive))]"
                          onClick={() => {
                            setIxDeleting(d);
                            setIxDeleteOpen(true);
                          }}
                        >
                          <IconTrash className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      {/* CF 域名列表 */}
      <div className="space-y-4">
        {loading ? (
          Array.from({ length: 3 }).map((_, i) => <SkeletonCard key={i} />)
        ) : domains.length === 0 ? (
          <Card>
            <CardContent className="flex h-40 items-center justify-center text-[hsl(var(--muted-foreground))]">
              暂无域名，点击右上角添加
            </CardContent>
          </Card>
        ) : (
          domains.map((d) => (
            <DomainCard
              key={d.id}
              domain={d}
              nodes={nodes}
              onDelete={(domain) => {
                setDeletingDomain(domain);
                setDeleteOpen(true);
              }}
            />
          ))
        )}
      </div>

      {/* 添加 CF 域名 Dialog */}
      <AddDomainDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        onSuccess={fetchDomains}
      />

      {/* 删除 CF 域名确认 */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={(open) => {
          setDeleteOpen(open);
          if (!open) setDeletingDomain(null);
        }}
        title="删除域名"
        description={
          <>
            确定要删除域名{" "}
            <span className="font-semibold text-[hsl(var(--foreground))]">
              {deletingDomain?.zone_name}
            </span>{" "}
            吗？删除后将无法管理该域名的 DNS 记录。
          </>
        }
        confirmLabel="删除"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />

      {/* IX 域名表单 Dialog */}
      <IXDomainFormDialog
        open={ixFormOpen}
        onOpenChange={(v) => {
          setIxFormOpen(v);
          if (!v) setIxEditing(null);
        }}
        editing={ixEditing}
        onSuccess={fetchDomains}
      />

      {/* 删除 IX 域名确认 */}
      <ConfirmDialog
        open={ixDeleteOpen}
        onOpenChange={(open) => {
          setIxDeleteOpen(open);
          if (!open) setIxDeleting(null);
        }}
        title="删除 IX 域名"
        description={
          <>
            确定要删除 IX 域名{" "}
            <span className="font-semibold text-[hsl(var(--foreground))]">
              {ixDeleting?.name}
            </span>{" "}
            吗？此操作不可撤销。
          </>
        }
        confirmLabel="删除"
        variant="destructive"
        loading={ixDeleteLoading}
        onConfirm={handleIxDelete}
      />
    </div>
  );
}
