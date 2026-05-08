import { useState, useEffect } from "react";
import { Link, Outlet, useRouterState, useNavigate } from "@tanstack/react-router";
import {
  Button,
  Separator,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui";
import { clearToken } from "@/lib/auth";
import { api } from "@/lib/api";
import { getTheme, toggleTheme, type Theme } from "@/lib/theme";
import { Toaster, toast } from "@/components/ui";

const navItems = [
  { to: "/panel/dashboard", label: "Dashboard", icon: BarChartIcon },
  { to: "/panel/users", label: "Users", icon: UsersIcon },
  { to: "/panel/user-groups", label: "User Groups", icon: GroupIcon },
  { to: "/panel/nodes", label: "Nodes", icon: ServerIcon },
  { to: "/panel/inbounds", label: "Inbounds", icon: ArrowDownIcon },
  { to: "/panel/outbounds", label: "Outbounds", icon: ArrowUpIcon },
  { to: "/panel/routerules", label: "Route Rules", icon: RouteRuleIcon },
  { to: "/panel/domains", label: "Domains", icon: DomainIcon },
  { to: "/panel/sniproxy", label: "NodeGate", icon: GlobeIcon },
  { to: "/panel/latency", label: "Latency", icon: LatencyIcon },
  { to: "/panel/audit", label: "Audit Logs", icon: ShieldIcon },
  { to: "/panel/plans", label: "Plans", icon: TagIcon },
  { to: "/panel/announcements", label: "Announcements", icon: AnnouncementIcon },
  { to: "/panel/tickets", label: "Tickets", icon: TicketIcon },
  { to: "/panel/settings", label: "Settings", icon: SettingsIcon },
] as const;

export default function RootLayout() {
  const router = useRouterState();
  const navigate = useNavigate();
  const currentPath = router.location.pathname;
  const [sidebarOpen, setSidebarOpen] = useState(false);

  useEffect(() => {
    const item = navItems.find(({ to }) => currentPath.startsWith(to));
    document.title = item ? `${item.label} — Pulse` : "Pulse";
  }, [currentPath]);
  const [theme, setTheme] = useState<Theme>(getTheme);
  const [updateChecking, setUpdateChecking] = useState(false);
  const [updateResult, setUpdateResult] = useState<{ has_update: boolean; latest: string; current: string } | null>(null);
  type UpdatePhase = "idle" | "downloading" | "restarting" | "done";
  const [updatePhase, setUpdatePhase] = useState<UpdatePhase>("idle");
  const [currentVersion, setCurrentVersion] = useState<string | null>(null);

  useEffect(() => {
    api.get<{ version: string }>("/system/info")
      .then((r) => setCurrentVersion(r.version))
      .catch(() => {});
    // 静默检查更新，有新版本时亮红点
    api.get<{ has_update: boolean; latest: string; current: string }>("/system/update/check")
      .then((r) => { if (r.has_update) setUpdateResult(r); })
      .catch(() => {});
  }, []);

  // Standalone pages — no sidebar
  if (
    currentPath === "/panel/login" ||
    currentPath === "/shop" ||
    currentPath.startsWith("/shop/") ||
    currentPath === "/stat" ||
    currentPath === "/user" ||
    currentPath.startsWith("/user/")
  ) {
    return <Outlet />;
  }

  async function checkUpdate() {
    setUpdateChecking(true);
    try {
      const r = await api.get<{ has_update: boolean; latest: string; current: string }>("/system/update/check");
      setUpdateResult(r);
      toast(r.has_update ? `发现新版本 ${r.latest}` : `已是最新版本 ${r.latest}`, "success");
    } catch {
      toast("检查更新失败", "error");
    } finally {
      setUpdateChecking(false);
    }
  }

  async function applyUpdate() {
    setUpdatePhase("downloading");
    try {
      await api.post("/system/update/apply", {});
      // 利用 healthz 推断阶段：
      //   apply 响应后 + healthz 仍通  → 下载/安装中（旧进程仍活着）
      //   healthz 开始失败             → systemctl restart 触发，服务重启中
      //   healthz 恢复                 → 新进程已就绪
      let wasDown = false;
      await new Promise<void>((resolve) => {
        const poll = setInterval(async () => {
          try {
            const res = await fetch("/healthz");
            if (res.ok) {
              if (wasDown) { clearInterval(poll); setUpdatePhase("done"); resolve(); }
            } else {
              if (!wasDown) { wasDown = true; setUpdatePhase("restarting"); }
            }
          } catch {
            if (!wasDown) { wasDown = true; setUpdatePhase("restarting"); }
          }
        }, 2000);
      });
      await new Promise((r) => setTimeout(r, 800));
      window.location.reload();
    } catch {
      toast("更新失败", "error");
      setUpdatePhase("idle");
    }
  }

  async function handleLogout() {
    try {
      await api.post("/auth/logout", {});
    } catch {
      // ignore — clear token anyway
    }
    clearToken();
    navigate({ to: "/panel/login" });
  }

  const sidebarContent = (
    <>
      {/* Logo */}
      <div className="flex h-14 items-center gap-2.5 px-5">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" className="h-7 w-7 shrink-0">
          <rect width="32" height="32" rx="7" fill="#18181b" />
          <polyline
            points="4,16 9,16 12,9 16,23 20,12 23,16 28,16"
            fill="none"
            stroke="#fafafa"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
        <span className="text-lg font-bold tracking-tight">Pulse</span>
      </div>

      <Separator />

      {/* Nav links */}
      <nav className="flex-1 overflow-y-auto px-3 py-4">
        <ul className="flex flex-col gap-1">
          {navItems.map(({ to, label, icon: Icon }) => {
            const active = currentPath.startsWith(to);
            return (
              <li key={to}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      asChild
                      className={`w-full justify-start gap-3 ${
                        active
                          ? "bg-[hsl(var(--accent))] text-[hsl(var(--accent-foreground))]"
                          : "text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]"
                      }`}
                    >
                      <Link to={to} onClick={() => setSidebarOpen(false)}>
                        <Icon className="h-4 w-4 shrink-0" />
                        {label}
                      </Link>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="right">{label}</TooltipContent>
                </Tooltip>
              </li>
            );
          })}
        </ul>
      </nav>

      <Separator />

      {/* Footer */}
      <div className="px-3 py-3">
        <Button
          variant="ghost"
          className="w-full justify-start gap-3 text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]"
          onClick={() => setTheme(toggleTheme())}
        >
          {theme === "dark" ? (
            <SunIcon className="h-4 w-4 shrink-0" />
          ) : (
            <MoonIcon className="h-4 w-4 shrink-0" />
          )}
          {theme === "dark" ? "浅色模式" : "深色模式"}
        </Button>
        <Button
          variant="ghost"
          className="w-full justify-start gap-3 text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]"
          onClick={updateResult?.has_update ? applyUpdate : checkUpdate}
          disabled={updateChecking || updatePhase !== "idle"}
        >
          <span className="relative shrink-0">
            <UpdateIcon className="h-4 w-4" />
            {updateResult?.has_update && updatePhase === "idle" && (
              <span className="absolute -top-0.5 -right-0.5 h-1.5 w-1.5 rounded-full bg-red-500" />
            )}
          </span>
          <span className="flex flex-col items-start">
            <span>{updatePhase !== "idle" ? "更新中…" : updateChecking ? "检查中…" : updateResult?.has_update ? `更新到 ${updateResult.latest}` : "检查更新"}</span>
            {currentVersion && <span className="text-[10px] opacity-50 font-mono">{currentVersion}</span>}
          </span>
        </Button>
        <Button
          variant="ghost"
          className="w-full justify-start gap-3 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--destructive))]"
          onClick={handleLogout}
        >
          <LogoutIcon className="h-4 w-4 shrink-0" />
          登出
        </Button>
      </div>
    </>
  );

  return (
    <TooltipProvider delayDuration={300}>
      <div className="flex h-screen w-screen overflow-hidden bg-[hsl(var(--background))] text-[hsl(var(--foreground))]">
        {/* 更新进度全屏 overlay */}
        {updatePhase !== "idle" && (
          <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/60 backdrop-blur-sm">
            <div className="flex flex-col items-center gap-5 rounded-2xl bg-[hsl(var(--card))] px-14 py-10 shadow-2xl">
              {updatePhase === "done" ? (
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/15 text-green-500">
                  <CheckCircleIcon className="h-7 w-7" />
                </div>
              ) : (
                <div className="h-10 w-10 animate-spin rounded-full border-4 border-[hsl(var(--border))] border-t-[hsl(var(--primary))]" />
              )}
              <div className="text-center">
                <p className="text-base font-semibold">
                  {updatePhase === "downloading" && "正在下载并安装新版本"}
                  {updatePhase === "restarting"  && "服务重启中"}
                  {updatePhase === "done"        && "更新完成"}
                </p>
                <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">
                  {updatePhase === "downloading" && "请勿关闭此页面，通常需要 30～60 秒"}
                  {updatePhase === "restarting"  && "新版本即将就绪，请稍候…"}
                  {updatePhase === "done"        && "正在刷新页面…"}
                </p>
              </div>
            </div>
          </div>
        )}
        {/* 移动端遮罩 */}
        {sidebarOpen && (
          <div
            className="fixed inset-0 z-40 bg-black/50 md:hidden"
            onClick={() => setSidebarOpen(false)}
          />
        )}

        {/* Sidebar — 移动端：fixed overlay；桌面端：static */}
        <aside
          className={`
            fixed inset-y-0 left-0 z-50 flex h-full w-56 flex-col border-r border-[hsl(var(--border))] bg-[hsl(var(--card))] transition-transform duration-200
            md:static md:z-auto md:translate-x-0 md:shrink-0
            ${sidebarOpen ? "translate-x-0" : "-translate-x-full"}
          `}
        >
          {sidebarContent}
        </aside>

        {/* Main content */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* 移动端顶栏 */}
          <header className="flex h-14 shrink-0 items-center gap-3 border-b border-[hsl(var(--border))] bg-[hsl(var(--card))] px-4 md:hidden">
            <button
              onClick={() => setSidebarOpen(true)}
              className="rounded-md p-1.5 text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--accent))] hover:text-[hsl(var(--accent-foreground))]"
              aria-label="打开菜单"
            >
              <HamburgerIcon className="h-5 w-5" />
            </button>
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" className="h-6 w-6 shrink-0">
              <rect width="32" height="32" rx="7" fill="#18181b" />
              <polyline
                points="4,16 9,16 12,9 16,23 20,12 23,16 28,16"
                fill="none"
                stroke="#fafafa"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
            <span className="text-base font-bold tracking-tight">Pulse</span>
          </header>

          <main className="flex-1 overflow-y-auto">
            <Outlet />
          </main>
        </div>
        <Toaster />
      </div>
    </TooltipProvider>
  );
}

/* ── Inline SVG icon components ────────────────────────────────── */

function BarChartIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <line x1="12" y1="20" x2="12" y2="10" />
      <line x1="18" y1="20" x2="18" y2="4" />
      <line x1="6" y1="20" x2="6" y2="16" />
    </svg>
  );
}

function UsersIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </svg>
  );
}

function ServerIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
      <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
      <line x1="6" y1="6" x2="6.01" y2="6" />
      <line x1="6" y1="18" x2="6.01" y2="18" />
    </svg>
  );
}

function ArrowDownIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <line x1="12" y1="5" x2="12" y2="19" />
      <polyline points="19 12 12 19 5 12" />
    </svg>
  );
}

function ArrowUpIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <line x1="12" y1="19" x2="12" y2="5" />
      <polyline points="5 12 12 5 19 12" />
    </svg>
  );
}

function RouteRuleIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="6" cy="19" r="3" />
      <path d="M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15" />
      <circle cx="18" cy="5" r="3" />
    </svg>
  );
}

function SettingsIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  );
}

function UpdateIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
  );
}

function LogoutIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  );
}

function GlobeIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="12" r="10" />
      <line x1="2" y1="12" x2="22" y2="12" />
      <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
    </svg>
  );
}

function DomainIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" />
    </svg>
  );
}

function TagIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z" />
      <line x1="7" y1="7" x2="7.01" y2="7" />
    </svg>
  );
}

function HamburgerIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <line x1="3" y1="6" x2="21" y2="6" />
      <line x1="3" y1="12" x2="21" y2="12" />
      <line x1="3" y1="18" x2="21" y2="18" />
    </svg>
  );
}

function SunIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="12" cy="12" r="4" />
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

function MoonIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  );
}

function AnnouncementIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M22 8s-4 2-10 2S2 8 2 8v8s4-2 10-2 10 2 10 2V8z" />
      <line x1="2" y1="12" x2="22" y2="12" />
    </svg>
  );
}
function TicketIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M2 9a3 3 0 0 1 0 6v2a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-2a3 3 0 0 1 0-6V7a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2Z" />
      <path d="M13 5v2" />
      <path d="M13 17v2" />
      <path d="M13 11v2" />
    </svg>
  );
}

function GroupIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <rect x="2" y="7" width="9" height="9" rx="1" />
      <rect x="13" y="7" width="9" height="9" rx="1" />
      <path d="M6.5 7V5a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1v2" />
    </svg>
  );
}


function LatencyIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
    </svg>
  );
}

function ShieldIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      <line x1="12" y1="8" x2="12" y2="12" />
      <line x1="12" y1="16" x2="12.01" y2="16" />
    </svg>
  );
}

function CheckCircleIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
      <polyline points="22 4 12 14.01 9 11.01" />
    </svg>
  );
}
