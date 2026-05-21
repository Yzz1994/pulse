import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { getTheme } from "@/lib/theme";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  Button,
  Badge,
  Separator,
} from "@/components/ui";
import { ScrollArea } from "@/components/ui/scroll-area";
import { api } from "@/lib/api";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";
import { getToken } from "@/lib/auth";
import { marked } from "marked";
import MDEditor, { commands } from "@uiw/react-md-editor";

marked.setOptions({ breaks: true });

interface Ticket {
  id: string;
  user_id: string;
  username: string;
  title: string;
  status: "open" | "replied" | "closed";
  created_at: string;
  updated_at: string;
}

interface Message {
  id: string;
  ticket_id: string;
  content: string;
  is_admin: boolean;
  created_at: string;
}

interface Image {
  id: string;
  ticket_id: string;
  filename: string;
  stored_name: string;
  size: number;
  created_at: string;
}



const statusVariant: Record<string, "default" | "secondary" | "outline" | "destructive"> = {
  open: "destructive",
  replied: "default",
  closed: "secondary",
};

export default function TicketsPage() {
  const { t } = useTranslation();
  const statusLabel: Record<string, string> = {
    open: t("tickets.pending"),
    replied: t("tickets.replied"),
    closed: t("tickets.closed"),
  };
  const theme = useMemo(() => getTheme(), []);

  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<string>("all");

  // 详情视图
  const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [images, setImages] = useState<Image[]>([]);
  const [replyContent, setReplyContent] = useState("");
  const [replying, setReplying] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const handleAuthError = useAuthErrorHandler();

  const fetchTickets = useCallback(() => {
    setLoading(true);
    api
      .get<{ tickets: Ticket[] }>("/tickets")
      .then((data) => setTickets(data.tickets ?? []))
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setLoading(false));
  }, [handleAuthError]);

  useEffect(() => {
    fetchTickets();
  }, [fetchTickets]);

  function openTicket(ticket: Ticket) {
    setSelectedTicket(ticket);
    setReplyContent("");
    api
      .get<{ ticket: Ticket; messages: Message[]; images: Image[] }>(`/tickets/${ticket.id}`)
      .then((data) => {
        setSelectedTicket(data.ticket);
        setMessages(data.messages ?? []);
        setImages(data.images ?? []);
      })
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  function sendReply() {
    if (!selectedTicket || !replyContent.trim()) return;
    setReplying(true);
    api
      .post<Message>(`/tickets/${selectedTicket.id}/reply`, { content: replyContent })
      .then((msg) => {
        setMessages((prev) => [...prev, msg]);
        setReplyContent("");
        setSelectedTicket((t) => t ? { ...t, status: "replied" } : t);
        setTickets((prev) =>
          prev.map((t) => t.id === selectedTicket.id ? { ...t, status: "replied", updated_at: new Date().toISOString() } : t)
        );
      })
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setReplying(false));
  }

  function closeTicket() {
    if (!selectedTicket) return;
    api
      .post<unknown>(`/tickets/${selectedTicket.id}/close`, {})
      .then(() => {
        setSelectedTicket((t) => t ? { ...t, status: "closed" } : t);
        setTickets((prev) =>
          prev.map((t) => t.id === selectedTicket.id ? { ...t, status: "closed" } : t)
        );
      })
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  function reopenTicket() {
    if (!selectedTicket) return;
    api
      .post<unknown>(`/tickets/${selectedTicket.id}/reopen`, {})
      .then(() => {
        setSelectedTicket((t) => t ? { ...t, status: "open" } : t);
        setTickets((prev) =>
          prev.map((t) => t.id === selectedTicket.id ? { ...t, status: "open" } : t)
        );
      })
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  async function uploadImage(file: File) {
    if (!selectedTicket) return;
    setUploading(true);
    try {
      const form = new FormData();
      form.append("file", file);
      const token = getToken();
      const res = await fetch(`/v1/tickets/${selectedTicket.id}/images`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: form,
      });
      if (!res.ok) throw new Error("upload failed");
      const img: Image = await res.json();
      setImages((prev) => [...prev, img]);
      // 将图片 markdown 链接插入回复框
      setReplyContent((prev) =>
        prev + (prev ? "\n" : "") + `![${img.filename}](/v1/uploads/tickets/${img.stored_name})`
      );
    } catch (err) {
      if (handleAuthError(err)) return;
    } finally {
      setUploading(false);
    }
  }

  const filtered = filter === "all" ? tickets : tickets.filter((t) => t.status === filter);

  // 列表视图
  if (!selectedTicket) {
    return (
      <div className="p-4 sm:p-6 lg:p-8">
        <h1 className="mb-6 text-2xl font-bold text-[hsl(var(--foreground))]">{t("tickets.title")}</h1>

        <div className="max-w-4xl space-y-4">
          {/* 状态筛选 */}
          <div className="flex gap-2">
            {[
              { key: "all", label: t("tickets.all") },
              { key: "open", label: t("tickets.pending") },
              { key: "replied", label: t("tickets.replied") },
              { key: "closed", label: t("tickets.closed") },
            ].map((f) => (
              <Button
                key={f.key}
                size="sm"
                variant={filter === f.key ? "default" : "outline"}
                onClick={() => setFilter(f.key)}
              >
                {f.label}
                {f.key !== "all" && (
                  <span className="ml-1 text-xs opacity-70">
                    {tickets.filter((t) => t.status === f.key).length}
                  </span>
                )}
              </Button>
            ))}
          </div>

          <Card>
            <CardContent className="pt-4">
              {loading ? (
                <div className="space-y-2">
                  {[1, 2, 3].map((i) => (
                    <div key={i} className="h-12 animate-pulse rounded-lg bg-[hsl(var(--muted))]" />
                  ))}
                </div>
              ) : filtered.length === 0 ? (
                <p className="text-center text-sm text-[hsl(var(--muted-foreground))] py-8">{t("tickets.noTickets")}</p>
              ) : (
                <div className="space-y-1">
                  {filtered.map((t) => (
                    <button
                      key={t.id}
                      onClick={() => openTicket(t)}
                      className="w-full rounded-lg border border-[hsl(var(--border))] p-3 text-left hover:bg-[hsl(var(--accent))] transition-colors"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2 min-w-0">
                          <Badge variant={statusVariant[t.status]}>{statusLabel[t.status]}</Badge>
                          <span className="text-sm font-medium truncate">{t.title}</span>
                        </div>
                        <div className="flex items-center gap-2 shrink-0 text-xs text-[hsl(var(--muted-foreground))]">
                          <span>{t.username}</span>
                          <span>{new Date(t.updated_at).toLocaleString("zh-CN")}</span>
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  // 详情视图
  return (
    <div className="p-4 sm:p-6 lg:p-8">
      <div className="max-w-4xl">
        {/* 顶部栏 */}
        <div className="mb-4 flex items-center gap-3">
          <Button size="sm" variant="outline" onClick={() => { setSelectedTicket(null); fetchTickets(); }}>
            {t("tickets.goBack")}
          </Button>
          <h1 className="text-xl font-bold truncate">{selectedTicket.title}</h1>
          <Badge variant={statusVariant[selectedTicket.status]}>
            {statusLabel[selectedTicket.status]}
          </Badge>
          <span className="text-sm text-[hsl(var(--muted-foreground))]">
            {selectedTicket.username}
          </span>
          <div className="ml-auto flex gap-2">
            {selectedTicket.status !== "closed" ? (
              <Button size="sm" variant="outline" onClick={closeTicket}>{t("tickets.closeTicket")}</Button>
            ) : (
              <Button size="sm" variant="outline" onClick={reopenTicket}>{t("tickets.reopen")}</Button>
            )}
          </div>
        </div>

        <Separator className="mb-4" />

        {/* 消息对话流 */}
        <Card>
          <CardContent className="pt-4">
            <ScrollArea className="max-h-[60vh]">
              <div className="space-y-4">
              {messages.map((m) => (
                <div
                  key={m.id}
                  className={`flex ${m.is_admin ? "justify-end" : "justify-start"}`}
                >
                  <div
                    className={`max-w-[75%] rounded-lg px-4 py-3 ${
                      m.is_admin
                        ? "bg-[hsl(var(--primary))] text-[hsl(var(--primary-foreground))]"
                        : "bg-[hsl(var(--muted))]"
                    }`}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-xs font-medium">
                        {m.is_admin ? t("tickets.admin") : selectedTicket.username}
                      </span>
                      <span className="text-xs opacity-60">
                        {new Date(m.created_at).toLocaleString("zh-CN")}
                      </span>
                    </div>
                    <div
                      className="prose prose-sm max-w-none dark:prose-invert [&_img]:max-w-full [&_img]:max-h-48 [&_img]:rounded [&_img]:object-contain"
                      dangerouslySetInnerHTML={{ __html: marked.parse(m.content) as string }}
                    />
                  </div>
                </div>
              ))}
              <div ref={messagesEndRef} />
              </div>
            </ScrollArea>
          </CardContent>
        </Card>

        {/* 回复框 */}
        {selectedTicket.status !== "closed" && (
          <Card className="mt-4">
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("tickets.reply")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4 space-y-3" data-color-mode={theme === "dark" ? "dark" : "light"}>
              <MDEditor
                value={replyContent}
                onChange={(v) => setReplyContent(v ?? "")}
                height={180}
                preview="edit"
                visibleDragbar={false}
                commands={[
                  commands.bold, commands.italic, commands.strikethrough,
                  commands.divider,
                  commands.quote, commands.code,
                  commands.divider,
                  commands.link,
                  commands.divider,
                  {
                    name: "upload-image",
                    keyCommand: "upload-image",
                    buttonProps: { "aria-label": t("tickets.uploadImage"), title: t("tickets.uploadImage") },
                    icon: (
                      <svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor">
                        <path d="M6.002 5.5a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0Z" />
                        <path d="M1.5 2A1.5 1.5 0 0 0 0 3.5v9A1.5 1.5 0 0 0 1.5 14h13a1.5 1.5 0 0 0 1.5-1.5v-9A1.5 1.5 0 0 0 14.5 2h-13Zm13 1a.5.5 0 0 1 .5.5v6l-3.775-1.947a.5.5 0 0 0-.577.093l-3.71 3.71-2.66-1.772a.5.5 0 0 0-.63.062L1.002 12v.54A.505.505 0 0 1 1 12.5v-9a.5.5 0 0 1 .5-.5h13Z" />
                      </svg>
                    ),
                    execute: () => { fileInputRef.current?.click(); },
                  },
                ]}
                extraCommands={[]}
              />
              <input
                ref={fileInputRef}
                type="file"
                accept=".jpg,.jpeg,.png,.gif,.webp"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) uploadImage(f);
                  e.target.value = "";
                }}
              />
              <div className="flex items-center gap-3">
                <Button size="sm" onClick={sendReply} disabled={replying || !replyContent.trim()}>
                  {replying ? t("tickets.sending") : t("tickets.sendReply")}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

      </div>
    </div>
  );
}
