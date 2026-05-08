import { useEffect, useState, useCallback } from "react";
import {
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
  MultiSelect,
  ConfirmDialog,
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  toast,
} from "@/components/ui";
import { api } from "@/lib/api";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";
import { hostSubName } from "@/lib/format";
import type {
  UserGroup,
  UserGroupsResponse,
  UserGroupMember,
  UserGroupMembersResponse,
  Inbound,
  InboundsResponse,
  Node,
  NodesResponse,
  Host,
  HostsResponse,
  Plan,
  PlansResponse,
  User,
  UsersResponse,
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

function IconSync({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className={className}>
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
  );
}

// ── 新建/编辑用户组 Dialog ────────────────────────────────────────

interface GroupFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group: UserGroup | null; // null = 新建
  allInbounds: Inbound[];
  allNodes: Node[];
  allHosts: Host[];
  onSaved: () => void;
  handleAuthError: (err: unknown) => boolean;
}

function GroupFormDialog({
  open,
  onOpenChange,
  group,
  allInbounds,
  allNodes,
  allHosts,
  onSaved,
  handleAuthError,
}: GroupFormDialogProps) {
  const [name, setName] = useState("");
  const [remark, setRemark] = useState("");
  const [selectedInboundIds, setSelectedInboundIds] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setName(group?.name ?? "");
      setRemark(group?.remark ?? "");
      setSelectedInboundIds(
        group?.inbound_ids ? group.inbound_ids.split(",").filter(Boolean) : []
      );
      setError("");
    }
  }, [open, group]);

  const inboundOptions = allInbounds.map((ib) => {
    const nodeName = allNodes.find((n) => n.id === ib.node_id)?.name ?? ib.node_id;
    const primaryHost = allHosts.find((h) => h.inbound_id === ib.id);
    const subName = primaryHost ? hostSubName(primaryHost, ib.traffic_rate) : "";
    const displayName = subName || ib.tag || `${ib.protocol}:${ib.port}`;
    return {
      value: ib.id,
      triggerLabel: `${nodeName} · ${ib.protocol} · ${displayName}`,
      label: (
        <span>
          <span className="text-[hsl(var(--muted-foreground))]">{nodeName} · {ib.protocol} · </span>
          <span className="font-medium">{displayName}</span>
        </span>
      ),
    };
  });

  async function handleSubmit() {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("用户组名称不能为空");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      if (group) {
        await api.put(`/user-groups/${group.id}`, {
          name: trimmedName,
          remark: remark.trim(),
          inbound_ids: selectedInboundIds.join(","),
        });
        toast.success("用户组已更新");
      } else {
        await api.post("/user-groups", {
          name: trimmedName,
          remark: remark.trim(),
          inbound_ids: selectedInboundIds.join(","),
        });
        toast.success("用户组已创建");
      }
      onOpenChange(false);
      onSaved();
    } catch (err) {
      if (handleAuthError(err)) return;
      setError(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{group ? "编辑用户组" : "新建用户组"}</DialogTitle>
          <DialogDescription>
            {group ? `修改用户组「${group.name}」的信息。` : "创建一个新的用户组。"}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          {error && (
            <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
              {error}
            </div>
          )}
          <div className="grid gap-2">
            <Label htmlFor="group-name">
              名称 <span className="text-[hsl(var(--destructive))]">*</span>
            </Label>
            <Input
              id="group-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="输入用户组名称"
              onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
              autoFocus
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="group-remark">备注</Label>
            <Input
              id="group-remark"
              value={remark}
              onChange={(e) => setRemark(e.target.value)}
              placeholder="可选备注"
            />
          </div>
          <div className="grid gap-2">
            <div className="flex items-center justify-between">
              <Label>关联 Inbound</Label>
              {allInbounds.length > 0 && (
                <div className="flex gap-2 text-xs">
                  <button
                    type="button"
                    className="text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                    onClick={() => setSelectedInboundIds(allInbounds.map((ib) => ib.id))}
                  >
                    全选
                  </button>
                  <span className="text-[hsl(var(--border))]">·</span>
                  <button
                    type="button"
                    className="text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                    onClick={() => setSelectedInboundIds([])}
                  >
                    全不选
                  </button>
                </div>
              )}
            </div>
            {allInbounds.length === 0 ? (
              <p className="text-sm text-[hsl(var(--muted-foreground))]">暂无可用 Inbound</p>
            ) : (
              <MultiSelect
                value={selectedInboundIds}
                onChange={setSelectedInboundIds}
                options={inboundOptions}
                placeholder="选择 Inbound..."
                countLabel="已选 {n} 个 Inbound"
              />
            )}
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" disabled={submitting}>
              取消
            </Button>
          </DialogClose>
          <Button type="button" disabled={submitting} onClick={handleSubmit}>
            {submitting ? "保存中…" : "确认"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── 应用套餐 Dialog ───────────────────────────────────────────────

interface ApplyPlanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groupId: string;
  plans: Plan[];
  handleAuthError: (err: unknown) => boolean;
}

function ApplyPlanDialog({ open, onOpenChange, groupId, plans, handleAuthError }: ApplyPlanDialogProps) {
  const [selectedPlanId, setSelectedPlanId] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setSelectedPlanId("");
      setError("");
    }
  }, [open]);

  async function handleSubmit() {
    if (!selectedPlanId) {
      setError("请选择套餐");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      await api.post(`/user-groups/${groupId}/apply-plan`, { plan_id: selectedPlanId });
      toast.success("套餐已应用到用户组");
      onOpenChange(false);
    } catch (err) {
      if (handleAuthError(err)) return;
      setError(err instanceof Error ? err.message : "操作失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>应用套餐</DialogTitle>
          <DialogDescription>为用户组内所有成员应用所选套餐。</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          {error && (
            <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
              {error}
            </div>
          )}
          <div className="grid gap-2">
            <Label>套餐</Label>
            <Select value={selectedPlanId} onValueChange={setSelectedPlanId}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="选择套餐..." />
              </SelectTrigger>
              <SelectContent>
                {plans.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" disabled={submitting}>
              取消
            </Button>
          </DialogClose>
          <Button type="button" disabled={submitting} onClick={handleSubmit}>
            {submitting ? "应用中…" : "确认应用"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── 添加成员 Dialog ───────────────────────────────────────────────

interface AddMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groupId: string;
  existingMemberIds: string[];
  onAdded: () => void;
  handleAuthError: (err: unknown) => boolean;
}

function AddMemberDialog({ open, onOpenChange, groupId, existingMemberIds, onAdded, handleAuthError }: AddMemberDialogProps) {
  const [users, setUsers] = useState<User[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setSelectedIds([]);
    setLoading(true);
    api.get<UsersResponse>("/users")
      .then((r) => setUsers(r.users ?? []))
      .catch((err) => { if (!handleAuthError(err)) toast.error("加载用户列表失败"); })
      .finally(() => setLoading(false));
  }, [open, handleAuthError]);

  const options = users
    .filter((u) => !existingMemberIds.includes(u.id))
    .map((u) => ({ value: u.id, label: u.username, triggerLabel: u.username }));

  async function handleSubmit() {
    if (!selectedIds.length) return;
    setSubmitting(true);
    try {
      await Promise.all(
        selectedIds.map((uid) => api.post(`/user-groups/${groupId}/members`, { user_id: uid }))
      );
      toast.success(`已添加 ${selectedIds.length} 名成员`);
      onOpenChange(false);
      onAdded();
    } catch (err) {
      if (!handleAuthError(err)) toast.error(err instanceof Error ? err.message : "添加失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>添加成员</DialogTitle>
          <DialogDescription>选择要加入该用户组的用户。</DialogDescription>
        </DialogHeader>
        <div className="py-2">
          {loading ? (
            <div className="py-4 text-center text-xs text-[hsl(var(--muted-foreground))]">加载中…</div>
          ) : (
            <MultiSelect
              value={selectedIds}
              onChange={setSelectedIds}
              options={options}
              placeholder={options.length === 0 ? "所有用户已在组内" : "搜索并选择用户…"}
              countLabel="已选 {n} 名用户"
            />
          )}
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" disabled={submitting}>取消</Button>
          </DialogClose>
          <Button type="button" disabled={!selectedIds.length || submitting || loading} onClick={handleSubmit}>
            {submitting ? "添加中…" : `添加${selectedIds.length > 0 ? `（${selectedIds.length}）` : ""}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── 成员管理 Sheet 抽屉 ───────────────────────────────────────────

interface MemberSheetProps {
  group: UserGroup | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  allPlans: Plan[];
  handleAuthError: (err: unknown) => boolean;
}

function MemberSheet({ group, open, onOpenChange, allPlans, handleAuthError }: MemberSheetProps) {
  const [members, setMembers] = useState<UserGroupMember[]>([]);
  const [membersLoading, setMembersLoading] = useState(false);
  const [addMemberOpen, setAddMemberOpen] = useState(false);
  const [applyPlanOpen, setApplyPlanOpen] = useState(false);

  // 删除成员 confirm
  const [deleteMemberConfirm, setDeleteMemberConfirm] = useState(false);
  const [deletingMember, setDeletingMember] = useState<UserGroupMember | null>(null);

  const fetchMembers = useCallback(() => {
    if (!group) return;
    setMembersLoading(true);
    api
      .get<UserGroupMembersResponse>(`/user-groups/${group.id}/members`)
      .then((res) => setMembers(res.members ?? []))
      .catch((err) => {
        if (handleAuthError(err)) return;
        toast.error(err instanceof Error ? err.message : "加载成员失败");
      })
      .finally(() => setMembersLoading(false));
  }, [group, handleAuthError]);

  // 打开 Sheet 时拉成员列表
  useEffect(() => {
    if (open && group) {
      fetchMembers();
    } else {
      setMembers([]);
    }
  }, [open, group, fetchMembers]);

  async function handleRemoveMember() {
    if (!deletingMember || !group) return;
    try {
      await api.del(`/user-groups/${group.id}/members/${deletingMember.user_id}`);
      toast.success("成员已移除");
      setDeleteMemberConfirm(false);
      setDeletingMember(null);
      fetchMembers();
    } catch (err) {
      if (handleAuthError(err)) return;
      toast.error(err instanceof Error ? err.message : "移除失败");
    }
  }

  if (!group) return null;

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent className="flex flex-col">
          <SheetHeader>
            <div className="flex items-center justify-between pr-8">
              <SheetTitle>{group.name}</SheetTitle>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 text-xs"
                  onClick={() => setApplyPlanOpen(true)}
                >
                  应用套餐
                </Button>
                <Button
                  size="sm"
                  className="h-7 text-xs"
                  onClick={() => setAddMemberOpen(true)}
                >
                  <IconPlus className="mr-1 h-3 w-3" />
                  添加成员
                </Button>
              </div>
            </div>
            <SheetDescription>
              {group.remark ? group.remark : "管理该用户组的成员"}
            </SheetDescription>
          </SheetHeader>

          {/* 成员列表 */}
          <div className="flex-1 overflow-y-auto px-6 py-4">
            {membersLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <div key={i} className="flex items-center justify-between rounded-md px-3 py-2.5 border border-[hsl(var(--border))]">
                    <div className="h-4 w-32 animate-pulse rounded bg-[hsl(var(--muted))]" />
                    <div className="h-7 w-7 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  </div>
                ))}
              </div>
            ) : members.length === 0 ? (
              <div className="flex h-32 items-center justify-center text-sm text-[hsl(var(--muted-foreground))]">
                暂无成员，点击「添加成员」加入
              </div>
            ) : (
              <div className="space-y-1">
                {members.map((m) => (
                  <div
                    key={m.user_id}
                    className="flex items-center justify-between rounded-md px-3 py-2 hover:bg-[hsl(var(--accent))]/50"
                  >
                    <span className="text-sm font-medium">{m.username}</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 w-7 p-0 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--destructive))]"
                      onClick={() => { setDeletingMember(m); setDeleteMemberConfirm(true); }}
                    >
                      <IconTrash className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </SheetContent>
      </Sheet>

      {/* 添加成员 Dialog */}
      <AddMemberDialog
        open={addMemberOpen}
        onOpenChange={setAddMemberOpen}
        groupId={group.id}
        existingMemberIds={members.map((m) => m.user_id)}
        onAdded={fetchMembers}
        handleAuthError={handleAuthError}
      />

      {/* 应用套餐 Dialog */}
      <ApplyPlanDialog
        open={applyPlanOpen}
        onOpenChange={setApplyPlanOpen}
        groupId={group.id}
        plans={allPlans}
        handleAuthError={handleAuthError}
      />

      {/* 移除成员 Confirm */}
      <ConfirmDialog
        open={deleteMemberConfirm}
        onOpenChange={setDeleteMemberConfirm}
        title="移除成员"
        description={`确定要将用户「${deletingMember?.username}」从该用户组中移除吗？`}
        confirmLabel="移除"
        onConfirm={handleRemoveMember}
      />
    </>
  );
}

// ── 主页面 ────────────────────────────────────────────────────────

export default function UserGroupsPage() {
  const [groups, setGroups] = useState<UserGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // 外部数据
  const [allInbounds, setAllInbounds] = useState<Inbound[]>([]);
  const [allNodes, setAllNodes] = useState<Node[]>([]);
  const [allHosts, setAllHosts] = useState<Host[]>([]);
  const [allPlans, setAllPlans] = useState<Plan[]>([]);

  // 新建/编辑 Dialog
  const [formOpen, setFormOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<UserGroup | null>(null);

  // 删除 confirm
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletingGroup, setDeletingGroup] = useState<UserGroup | null>(null);
  const [deleting, setDeleting] = useState(false);

  // 成员管理 Sheet
  const [sheetOpen, setSheetOpen] = useState(false);
  const [sheetGroup, setSheetGroup] = useState<UserGroup | null>(null);

  // 行级同步状态
  const [syncingIds, setSyncingIds] = useState<Set<string>>(new Set());

  const handleAuthError = useAuthErrorHandler();

  const fetchGroups = useCallback(() => {
    setLoading(true);
    setError(null);
    api
      .get<UserGroupsResponse>("/user-groups")
      .then((res) => setGroups(res.user_groups ?? []))
      .catch((err) => {
        if (handleAuthError(err)) return;
        setError(err instanceof Error ? err.message : "加载失败");
      })
      .finally(() => setLoading(false));
  }, [handleAuthError]);

  // 加载所有外部数据
  useEffect(() => {
    fetchGroups();

    api
      .get<InboundsResponse>("/inbounds")
      .then((res) => setAllInbounds(res.inbounds ?? []))
      .catch((err) => { handleAuthError(err); });

    api
      .get<NodesResponse>("/nodes")
      .then((res) => setAllNodes(res.nodes ?? []))
      .catch((err) => { handleAuthError(err); });

    api
      .get<{ hosts: Host[] }>("/hosts")
      .then((res) => setAllHosts(res.hosts ?? []))
      .catch((err) => { handleAuthError(err); });

    api
      .get<PlansResponse>("/plans")
      .then((res) => setAllPlans(res.plans ?? []))
      .catch((err) => { handleAuthError(err); });
  }, [handleAuthError]);

  function openCreateDialog() {
    setEditingGroup(null);
    setFormOpen(true);
  }

  function openEditDialog(group: UserGroup) {
    setEditingGroup(group);
    setFormOpen(true);
  }

  function openDeleteDialog(group: UserGroup) {
    setDeletingGroup(group);
    setDeleteOpen(true);
  }

  function openMemberSheet(group: UserGroup) {
    setSheetGroup(group);
    setSheetOpen(true);
  }

  async function handleDelete() {
    if (!deletingGroup) return;
    setDeleting(true);
    try {
      await api.del(`/user-groups/${deletingGroup.id}`);
      toast.success(`用户组「${deletingGroup.name}」已删除`);
      setDeleteOpen(false);
      setDeletingGroup(null);
      fetchGroups();
    } catch (err) {
      if (handleAuthError(err)) return;
      toast.error(err instanceof Error ? err.message : "删除失败");
    } finally {
      setDeleting(false);
    }
  }

  async function handleSync(group: UserGroup, e: React.MouseEvent) {
    e.stopPropagation();
    setSyncingIds((prev) => new Set(prev).add(group.id));
    try {
      const res = await api.post<{ affected_nodes: string[] }>(`/user-groups/${group.id}/sync`, {});
      const count = (res.affected_nodes ?? []).length;
      toast.success(`同步完成，影响 ${count} 个节点`);
    } catch (err) {
      if (handleAuthError(err)) return;
      toast.error(err instanceof Error ? err.message : "同步失败");
    } finally {
      setSyncingIds((prev) => {
        const next = new Set(prev);
        next.delete(group.id);
        return next;
      });
    }
  }

  return (
    <div className="flex h-full flex-col p-4 sm:p-6 lg:p-8">
      {/* 页面标题 */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-2xl font-bold text-[hsl(var(--foreground))]">User Groups</h1>
        <Button onClick={openCreateDialog}>
          <IconPlus className="mr-1.5 h-4 w-4" />
          + New Group
        </Button>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="mb-4 rounded-lg border border-[hsl(var(--destructive))] bg-[hsl(var(--destructive))]/10 px-4 py-3 text-sm text-[hsl(var(--destructive))]">
          {error}
          <Button variant="ghost" size="sm" className="ml-2" onClick={fetchGroups}>
            重试
          </Button>
        </div>
      )}

      {/* 用户组 Table */}
      <div className="rounded-lg border border-[hsl(var(--border))] overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称</TableHead>
              <TableHead>备注</TableHead>
              <TableHead className="text-center">成员数</TableHead>
              <TableHead className="text-center">Inbound 数</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell><div className="h-4 w-28 animate-pulse rounded bg-[hsl(var(--muted))]" /></TableCell>
                  <TableCell><div className="h-4 w-20 animate-pulse rounded bg-[hsl(var(--muted))]" /></TableCell>
                  <TableCell className="text-center"><div className="mx-auto h-4 w-8 animate-pulse rounded bg-[hsl(var(--muted))]" /></TableCell>
                  <TableCell className="text-center"><div className="mx-auto h-4 w-8 animate-pulse rounded bg-[hsl(var(--muted))]" /></TableCell>
                  <TableCell className="text-right"><div className="ml-auto h-7 w-24 animate-pulse rounded bg-[hsl(var(--muted))]" /></TableCell>
                </TableRow>
              ))
            ) : groups.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-32 text-center text-[hsl(var(--muted-foreground))]">
                  暂无用户组，点击「+ New Group」创建
                </TableCell>
              </TableRow>
            ) : (
              groups.map((group) => {
                const inboundCount = group.inbound_ids
                  ? group.inbound_ids.split(",").filter(Boolean).length
                  : 0;
                const isSyncing = syncingIds.has(group.id);

                return (
                  <TableRow
                    key={group.id}
                    className="cursor-pointer"
                    onClick={() => openMemberSheet(group)}
                  >
                    <TableCell className="font-medium">{group.name}</TableCell>
                    <TableCell className="text-[hsl(var(--muted-foreground))] text-sm">
                      {group.remark || "—"}
                    </TableCell>
                    <TableCell className="text-center text-sm">{group.member_count ?? "—"}</TableCell>
                    <TableCell className="text-center text-sm">{inboundCount}</TableCell>
                    <TableCell className="text-right">
                      {/* 阻止点击冒泡到行 */}
                      <div
                        className="flex items-center justify-end gap-1"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0"
                          title="同步"
                          disabled={isSyncing}
                          onClick={(e) => handleSync(group, e)}
                        >
                          <IconSync className={`h-3.5 w-3.5 ${isSyncing ? "animate-spin" : ""}`} />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0"
                          title="编辑"
                          onClick={(e) => { e.stopPropagation(); openEditDialog(group); }}
                        >
                          <IconEdit className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--destructive))]"
                          title="删除"
                          onClick={(e) => { e.stopPropagation(); openDeleteDialog(group); }}
                        >
                          <IconTrash className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* 成员管理 Sheet */}
      <MemberSheet
        group={sheetGroup}
        open={sheetOpen}
        onOpenChange={setSheetOpen}
        allPlans={allPlans}
        handleAuthError={handleAuthError}
      />

      {/* 新建/编辑 Dialog */}
      <GroupFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        group={editingGroup}
        allInbounds={allInbounds}
        allNodes={allNodes}
        allHosts={allHosts}
        onSaved={fetchGroups}
        handleAuthError={handleAuthError}
      />

      {/* 删除 Confirm */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={(v) => {
          setDeleteOpen(v);
          if (!v) setDeletingGroup(null);
        }}
        title="删除用户组"
        description={`确定要删除用户组「${deletingGroup?.name}」吗？此操作不可撤销。`}
        confirmLabel="删除"
        onConfirm={handleDelete}
      />
    </div>
  );
}
