import { useEffect, useState, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  Button,
  Input,
  Separator,
  ConfirmDialog,
} from "@/components/ui";
import { api, AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";

interface Announcement {
  id: string;
  title: string;
  content: string;
  enabled: boolean;
  created_at: string;
}

export default function AnnouncementsPage() {
  const navigate = useNavigate();

  const [anns, setAnns] = useState<Announcement[]>([]);
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState<string | null>(null);

  const [newTitle, setNewTitle] = useState("");
  const [newContent, setNewContent] = useState("");
  const [creating, setCreating] = useState(false);

  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const [editContent, setEditContent] = useState("");
  const [saving, setSaving] = useState(false);

  function handleAuthError(err: unknown): boolean {
    if (err instanceof AuthError) {
      clearToken();
      navigate({ to: "/panel/login" });
      return true;
    }
    return false;
  }

  const fetchAnnouncements = useCallback(() => {
    setLoading(true);
    api
      .get<{ announcements: Announcement[] }>("/announcements")
      .then((data) => setAnns(data.announcements ?? []))
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setLoading(false));
  }, [navigate]);

  useEffect(() => {
    fetchAnnouncements();
  }, [fetchAnnouncements]);

  function showMsg(text: string) {
    setMsg(text);
    setTimeout(() => setMsg(null), 2000);
  }

  function createAnnouncement() {
    setCreating(true);
    api
      .post<Announcement>("/announcements", { title: newTitle, content: newContent })
      .then((a) => {
        setAnns((prev) => [a, ...prev]);
        setNewTitle("");
        setNewContent("");
        showMsg("已创建");
      })
      .catch((err) => { if (handleAuthError(err)) return; showMsg("创建失败"); })
      .finally(() => setCreating(false));
  }

  function startEdit(a: Announcement) {
    setEditingId(a.id);
    setEditTitle(a.title);
    setEditContent(a.content);
  }

  function saveEdit(id: string) {
    setSaving(true);
    api
      .put<Announcement>(`/announcements/${id}`, { title: editTitle, content: editContent })
      .then((updated) => {
        setAnns((prev) => prev.map((a) => (a.id === id ? updated : a)));
        setEditingId(null);
        showMsg("已保存");
      })
      .catch((err) => { if (handleAuthError(err)) return; showMsg("保存失败"); })
      .finally(() => setSaving(false));
  }

  function deleteAnn(id: string) {
    setDeleteConfirmId(id);
  }

  function doDeleteAnn() {
    const id = deleteConfirmId;
    if (!id) return;
    setDeleteConfirmId(null);
    api
      .del<unknown>(`/announcements/${id}`)
      .then(() => setAnns((prev) => prev.filter((a) => a.id !== id)))
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  function activateAnn(id: string) {
    api
      .post<unknown>(`/announcements/${id}/activate`, {})
      .then(() => setAnns((prev) => prev.map((a) => ({ ...a, enabled: a.id === id }))))
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  function disableAnn(id: string) {
    api
      .post<unknown>(`/announcements/${id}/disable`, {})
      .then(() => setAnns((prev) => prev.map((a) => a.id === id ? { ...a, enabled: false } : a)))
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  return (
    <div className="p-4 sm:p-6 lg:p-8">
      <h1 className="mb-6 text-2xl font-bold text-[hsl(var(--foreground))]">公告管理</h1>

      <div className="max-w-2xl space-y-6">
        {/* 新建公告 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base font-medium">新建公告</CardTitle>
          </CardHeader>
          <Separator />
          <CardContent className="pt-4 space-y-3">
            <Input
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              placeholder="标题（可留空）"
            />
            <textarea
              value={newContent}
              onChange={(e) => setNewContent(e.target.value)}
              placeholder="内容（支持 Markdown）"
              rows={5}
              className="w-full resize-y rounded-lg border border-[hsl(var(--border))] bg-transparent px-3 py-2 font-mono text-sm text-[hsl(var(--foreground))] placeholder:text-[hsl(var(--muted-foreground))] focus:outline-none focus:ring-2 focus:ring-[hsl(var(--ring))]"
            />
            <div className="flex items-center gap-3">
              <Button size="sm" onClick={createAnnouncement} disabled={creating || !newContent.trim()}>
                {creating ? "创建中…" : "创建"}
              </Button>
              {msg && <span className="text-sm text-[hsl(var(--muted-foreground))]">{msg}</span>}
            </div>
          </CardContent>
        </Card>

        {/* 公告列表 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base font-medium">历史记录</CardTitle>
          </CardHeader>
          <Separator />
          <CardContent className="pt-4">
            {loading ? (
              <div className="space-y-2">
                {[1, 2, 3].map((i) => (
                  <div key={i} className="h-16 animate-pulse rounded-lg bg-[hsl(var(--muted))]" />
                ))}
              </div>
            ) : anns.length === 0 ? (
              <p className="text-center text-sm text-[hsl(var(--muted-foreground))] py-8">暂无公告</p>
            ) : (
              <div className="space-y-2">
                {anns.map((a) => (
                  <div key={a.id} className="rounded-lg border border-[hsl(var(--border))] p-3 space-y-2">
                    {editingId === a.id ? (
                      <div className="space-y-2">
                        <Input value={editTitle} onChange={(e) => setEditTitle(e.target.value)} placeholder="标题" />
                        <textarea
                          value={editContent}
                          onChange={(e) => setEditContent(e.target.value)}
                          rows={4}
                          className="w-full resize-y rounded-lg border border-[hsl(var(--border))] bg-transparent px-3 py-2 font-mono text-sm text-[hsl(var(--foreground))] focus:outline-none focus:ring-2 focus:ring-[hsl(var(--ring))]"
                        />
                        <div className="flex gap-2">
                          <Button size="sm" onClick={() => saveEdit(a.id)} disabled={saving}>
                            {saving ? "保存中…" : "保存"}
                          </Button>
                          <Button size="sm" variant="outline" onClick={() => setEditingId(null)}>取消</Button>
                        </div>
                      </div>
                    ) : (
                      <>
                        <div className="flex items-center justify-between gap-2">
                          <div className="flex items-center gap-2 min-w-0">
                            {a.enabled && (
                              <span className="shrink-0 rounded px-1.5 py-0.5 text-xs font-medium bg-[hsl(var(--primary))]/15 text-[hsl(var(--primary))]">
                                激活
                              </span>
                            )}
                            <span className="text-sm font-medium truncate">
                              {a.title || <span className="italic text-[hsl(var(--muted-foreground))]">无标题</span>}
                            </span>
                          </div>
                          <div className="flex shrink-0 items-center gap-1">
                            {a.enabled
                              ? <Button size="sm" variant="outline" onClick={() => disableAnn(a.id)}>禁用</Button>
                              : <Button size="sm" onClick={() => activateAnn(a.id)}>激活</Button>
                            }
                            <Button size="sm" variant="outline" onClick={() => startEdit(a)}>编辑</Button>
                            <Button size="sm" variant="destructive" onClick={() => deleteAnn(a.id)}>删除</Button>
                          </div>
                        </div>
                        {a.content && (
                          <p className="text-xs text-[hsl(var(--muted-foreground))] font-mono line-clamp-2">
                            {a.content}
                          </p>
                        )}
                        <p className="text-xs text-[hsl(var(--muted-foreground))]/50">
                          {new Date(a.created_at).toLocaleString("zh-CN")}
                        </p>
                      </>
                    )}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <ConfirmDialog
        open={deleteConfirmId !== null}
        onOpenChange={(open) => { if (!open) setDeleteConfirmId(null); }}
        title="确认删除"
        description="确定要删除此公告吗？此操作不可撤销。"
        confirmLabel="删除"
        onConfirm={doDeleteAnn}
      />
    </div>
  );
}
