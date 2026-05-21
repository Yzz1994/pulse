import { useEffect, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
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
import { api } from "@/lib/api";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";

interface Announcement {
  id: string;
  title: string;
  content: string;
  enabled: boolean;
  created_at: string;
}

export default function AnnouncementsPage() {
  const { t } = useTranslation();
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

  const handleAuthError = useAuthErrorHandler();

  const fetchAnnouncements = useCallback(() => {
    setLoading(true);
    api
      .get<{ announcements: Announcement[] }>("/announcements")
      .then((data) => setAnns(data.announcements ?? []))
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setLoading(false));
  }, [handleAuthError]);

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
        showMsg(t("announcements.created"));
      })
      .catch((err) => { if (handleAuthError(err)) return;         showMsg(t("common.createFailed")); })
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
        showMsg(t("announcements.saved"));
      })
      .catch((err) => { if (handleAuthError(err)) return; showMsg(t("common.saveFailed")); })
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
      <h1 className="mb-6 text-2xl font-bold text-[hsl(var(--foreground))]">{t("announcements.title")}</h1>

      <div className="max-w-2xl space-y-6">
        {/* 新建公告 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base font-medium">{t("announcements.newAnnouncement")}</CardTitle>
          </CardHeader>
          <Separator />
          <CardContent className="pt-4 space-y-3">
            <Input
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              placeholder={t("announcements.titlePlaceholder")}
            />
            <textarea
              value={newContent}
              onChange={(e) => setNewContent(e.target.value)}
              placeholder={t("announcements.contentPlaceholder")}
              rows={5}
              className="w-full resize-y rounded-lg border border-[hsl(var(--border))] bg-transparent px-3 py-2 font-mono text-sm text-[hsl(var(--foreground))] placeholder:text-[hsl(var(--muted-foreground))] focus:outline-none focus:ring-2 focus:ring-[hsl(var(--ring))]"
            />
            <div className="flex items-center gap-3">
              <Button size="sm" onClick={createAnnouncement} disabled={creating || !newContent.trim()}>
                {creating ? t("common.creating") : t("common.create")}
              </Button>
              {msg && <span className="text-sm text-[hsl(var(--muted-foreground))]">{msg}</span>}
            </div>
          </CardContent>
        </Card>

        {/* 公告列表 */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base font-medium">{t("announcements.historyTitle")}</CardTitle>
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
              <p className="text-center text-sm text-[hsl(var(--muted-foreground))] py-8">{t("announcements.noAnnouncements")}</p>
            ) : (
              <div className="space-y-2">
                {anns.map((a) => (
                  <div key={a.id} className="rounded-lg border border-[hsl(var(--border))] p-3 space-y-2">
                    {editingId === a.id ? (
                      <div className="space-y-2">
                         <Input value={editTitle} onChange={(e) => setEditTitle(e.target.value)} placeholder={t("announcements.editTitlePlaceholder")} />
                        <textarea
                          value={editContent}
                          onChange={(e) => setEditContent(e.target.value)}
                          rows={4}
                          className="w-full resize-y rounded-lg border border-[hsl(var(--border))] bg-transparent px-3 py-2 font-mono text-sm text-[hsl(var(--foreground))] focus:outline-none focus:ring-2 focus:ring-[hsl(var(--ring))]"
                        />
                        <div className="flex gap-2">
                          <Button size="sm" onClick={() => saveEdit(a.id)} disabled={saving}>
                             {saving ? t("common.saving") : t("common.save")}
                          </Button>
                           <Button size="sm" variant="outline" onClick={() => setEditingId(null)}>{t("common.cancel")}</Button>
                        </div>
                      </div>
                    ) : (
                      <>
                        <div className="flex items-center justify-between gap-2">
                          <div className="flex items-center gap-2 min-w-0">
                            {a.enabled && (
                              <span className="shrink-0 rounded px-1.5 py-0.5 text-xs font-medium bg-[hsl(var(--primary))]/15 text-[hsl(var(--primary))]">
                                {t("announcements.activated")}
                              </span>
                            )}
                            <span className="text-sm font-medium truncate">
                              {a.title || <span className="italic text-[hsl(var(--muted-foreground))]">{t("announcements.noTitle")}</span>}
                            </span>
                          </div>
                          <div className="flex shrink-0 items-center gap-1">
                            {a.enabled
                              ? <Button size="sm" variant="outline" onClick={() => disableAnn(a.id)}>{t("announcements.disableBtn")}</Button>
                              : <Button size="sm" onClick={() => activateAnn(a.id)}>{t("announcements.activateBtn")}</Button>
                            }
                            <Button size="sm" variant="outline" onClick={() => startEdit(a)}>{t("announcements.editBtn")}</Button>
                            <Button size="sm" variant="destructive" onClick={() => deleteAnn(a.id)}>{t("announcements.deleteBtn")}</Button>
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
        title={t("common.confirmDelete")}
        description={t("announcements.confirmDelete")}
        confirmLabel={t("common.delete")}
        onConfirm={doDeleteAnn}
      />
    </div>
  );
}
