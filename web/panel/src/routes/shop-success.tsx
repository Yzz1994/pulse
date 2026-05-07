import { useState, useEffect } from "react";
import { Card, CardContent, Button } from "@/components/ui";
import { getTheme, toggleTheme, type Theme } from "@/lib/theme";
import { copyText } from "@/lib/clipboard";

/* ── Types ────────────────────────────────────────────────────── */

interface OrderInfo {
  email: string;
  sub_url?: string;
  portal_url?: string;
}

/* ── Page ──────────────────────────────────────────────────────── */

function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(getTheme);
  return (
    <button
      onClick={() => setTheme(toggleTheme())}
      className="fixed right-4 top-4 z-50 rounded-md p-2 bg-[hsl(var(--card))] border border-[hsl(var(--border))] text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))] shadow-sm transition-colors"
      title={theme === "dark" ? "切换浅色模式" : "切换深色模式"}
    >
      {theme === "dark" ? (
        <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364-6.364l-.707.707M6.343 17.657l-.707.707M17.657 17.657l-.707-.707M6.343 6.343l-.707-.707M12 8a4 4 0 100 8 4 4 0 000-8z" />
        </svg>
      ) : (
        <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
        </svg>
      )}
    </button>
  );
}

export default function ShopSuccessPage() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [orderInfo, setOrderInfo] = useState<OrderInfo | null>(null);
  const [copied, setCopied] = useState(false);

  const sessionId = new URLSearchParams(window.location.search).get(
    "session_id",
  );

  useEffect(() => {
    if (!sessionId) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    // webhook 可能晚于页面加载到达，轮询直到拿到 sub_url 或超时
    const MAX_WAIT_MS = 30_000;
    const POLL_INTERVAL_MS = 2_000;
    const startedAt = Date.now();

    async function fetchOrderInfo() {
      try {
        const res = await fetch(
          `/v1/shop/order-info?session_id=${encodeURIComponent(sessionId!)}`,
        );
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.error ?? `HTTP ${res.status}`);
        }
        const data: OrderInfo = await res.json();
        if (cancelled) return;
        setOrderInfo(data);
        // 如果 sub_url 还没有（webhook 尚未处理），继续轮询
        if (!data.sub_url && Date.now() - startedAt < MAX_WAIT_MS) {
          setTimeout(fetchOrderInfo, POLL_INTERVAL_MS);
          return;
        }
      } catch (err) {
        if (!cancelled)
          setError(err instanceof Error ? err.message : "加载订单信息失败");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    fetchOrderInfo();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  function handleCopy() {
    if (!orderInfo?.sub_url) return;
    copyText(orderInfo.sub_url).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  function handleRetry() {
    setError("");
    setLoading(true);
    fetch(`/v1/shop/order-info?session_id=${encodeURIComponent(sessionId!)}`)
      .then(async (res) => {
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.error ?? `HTTP ${res.status}`);
        }
        return res.json();
      })
      .then((data: OrderInfo) => setOrderInfo(data))
      .catch((err) =>
        setError(err instanceof Error ? err.message : "加载订单信息失败"),
      )
      .finally(() => setLoading(false));
  }

  /* ── No session_id ──────────────────────────────────────────── */

  if (!sessionId) {
    return (
      <div className="flex h-screen overflow-y-auto items-center justify-center bg-[hsl(var(--background))] px-4">
        <ThemeToggle />
        <div className="w-full max-w-lg text-center">
          <div className="mb-8 flex flex-col items-center gap-3">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 32 32"
              className="h-12 w-12"
            >
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
          </div>
          <p className="mb-6 text-[hsl(var(--muted-foreground))]">
            缺少支付会话信息，请从商店重新发起购买。
          </p>
          <a href="/shop">
            <Button>返回商店</Button>
          </a>
        </div>
      </div>
    );
  }

  /* ── Loading ────────────────────────────────────────────────── */

  if (loading) {
    return (
      <div className="flex h-screen overflow-y-auto items-center justify-center bg-[hsl(var(--background))] px-4">
        <ThemeToggle />
        <div className="flex flex-col items-center gap-4">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-[hsl(var(--muted))] border-t-[hsl(var(--primary))]" />
          <p className="text-sm text-[hsl(var(--muted-foreground))]">
            正在加载订单信息…
          </p>
        </div>
      </div>
    );
  }

  /* ── Error ──────────────────────────────────────────────────── */

  if (error) {
    return (
      <div className="flex h-screen overflow-y-auto items-center justify-center bg-[hsl(var(--background))] px-4">
        <ThemeToggle />
        <div className="w-full max-w-lg text-center">
          <div className="mb-8 flex flex-col items-center gap-3">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 32 32"
              className="h-12 w-12"
            >
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
          </div>
          <div className="mx-auto mb-6 max-w-md rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-3 text-sm text-[hsl(var(--destructive))]">
            {error}
          </div>
          <div className="flex items-center justify-center gap-3">
            <Button variant="outline" onClick={handleRetry}>
              重试
            </Button>
            <a href="/shop">
              <Button variant="outline">返回商店</Button>
            </a>
          </div>
        </div>
      </div>
    );
  }

  /* ── Success ────────────────────────────────────────────────── */

  return (
    <div className="flex h-screen overflow-y-auto items-center justify-center bg-[hsl(var(--background))] px-4">
      <ThemeToggle />
      <div className="w-full max-w-lg">
        {/* Header */}
        <div className="mb-8 flex flex-col items-center gap-3">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 32 32"
            className="h-12 w-12"
          >
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
          <h1 className="text-2xl font-bold text-[hsl(var(--foreground))]">
            支付成功！
          </h1>
          <p className="text-sm text-[hsl(var(--muted-foreground))]">
            您的订阅已激活
          </p>
        </div>

        {/* Card */}
        <Card>
          <CardContent className="space-y-6 pt-6">
            {/* Email */}
            <div className="space-y-1.5">
              <p className="text-sm font-medium text-[hsl(var(--muted-foreground))]">
                账户邮箱
              </p>
              <p className="text-base font-medium text-[hsl(var(--foreground))]">
                {orderInfo?.email}
              </p>
            </div>

            {/* Subscription link */}
            {orderInfo?.sub_url && (
              <div className="space-y-2">
                <p className="text-sm font-medium text-[hsl(var(--muted-foreground))]">
                  订阅链接
                </p>
                <div className="flex gap-2">
                  <input
                    type="text"
                    readOnly
                    value={orderInfo.sub_url}
                    className="flex-1 rounded-md border border-[hsl(var(--border))] bg-[hsl(var(--muted))] px-3 py-2 text-sm text-[hsl(var(--foreground))] outline-none"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    onClick={handleCopy}
                  >
                    {copied ? "已复制" : "复制"}
                  </Button>
                </div>
              </div>
            )}

            {/* 等待 webhook 处理中 */}
            {orderInfo && !orderInfo.sub_url && (
              <div className="flex items-center gap-2 text-sm text-[hsl(var(--muted-foreground))]">
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-[hsl(var(--muted))] border-t-[hsl(var(--primary))]" />
                正在激活订阅，请稍候…
              </div>
            )}

            {/* Warning */}
            {orderInfo?.sub_url && (
              <div className="rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--muted))]/50 px-4 py-3">
                <p className="mb-1 text-sm font-medium text-[hsl(var(--foreground))]">
                  ⚠️ 重要提示
                </p>
                <p className="text-sm text-[hsl(var(--muted-foreground))]">
                  请妥善保管以下两个链接，它们是您访问服务的唯一凭证：
                </p>
                <ul className="mt-1 space-y-0.5 text-sm text-[hsl(var(--muted-foreground))]">
                  <li>• <span className="font-medium text-[hsl(var(--foreground))]">订阅链接</span>：导入代理客户端使用</li>
                  {orderInfo?.portal_url && (
                    <li>• <span className="font-medium text-[hsl(var(--foreground))]">个人主页</span>：{orderInfo.portal_url}</li>
                  )}
                </ul>
                <p className="mt-2 text-sm text-[hsl(var(--muted-foreground))]">
                  请勿将以上链接分享给他人。
                </p>
              </div>
            )}

            {/* Actions */}
            <div className="flex flex-col gap-2">
              {orderInfo?.portal_url && (
                <a href={orderInfo.portal_url} className="block">
                  <Button className="w-full">
                    前往个人主页
                  </Button>
                </a>
              )}
              <a href="/shop" className="block">
                <Button variant="outline" className="w-full">
                  返回商店
                </Button>
              </a>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
