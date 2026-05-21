import { useState, useEffect, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { getTheme, toggleTheme, type Theme } from "@/lib/theme";
import { api } from "../lib/api";
import { setToken } from "../lib/auth";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
  Label,
  Input,
  Button,
} from "@/components/ui";

export default function LoginPage() {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [discourseEnabled, setDiscourseEnabled] = useState(false);
  const [theme, setTheme] = useState<Theme>(getTheme);

  // Handle Discourse OAuth callback token
  useEffect(() => {
    const params = new URLSearchParams(window.location.hash.slice(1));
    const discourseToken = params.get("discourse_token");
    if (discourseToken) {
      setToken(discourseToken);
      window.location.replace("/panel/dashboard");
    }
  }, []);

  // Check if Discourse SSO is enabled
  useEffect(() => {
    fetch("/v1/auth/info")
      .then((res) => res.json())
      .then((data: { discourse_enabled?: boolean }) => {
        if (data.discourse_enabled) {
          setDiscourseEnabled(true);
        }
      })
      .catch(() => {
        // Ignore — Discourse button stays hidden
      });
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const data = await api.post<{ token: string; username: string }>(
        "/auth/login",
        { username, password },
      );
      if (!data?.token) {
        setError(t("login.loginDataError", { data: JSON.stringify(data) }));
        return;
      }
      setToken(data.token);
      // Full page reload to reinitialize router with auth state
      window.location.replace("/panel/dashboard");
      return;
    } catch (err) {
      if (err instanceof Error) {
        if (err.message.includes("429")) {
          setError(t("login.tooManyAttempts"));
        } else if (err.message.includes("401") || err.message.includes("invalid")) {
          setError(t("login.wrongCredentials"));
        } else {
          setError(err.message);
        }
      } else {
        setError(t("login.loginFailed"));
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex h-screen overflow-y-auto items-center justify-center bg-[hsl(var(--background))] px-4">
      <button
        onClick={() => setTheme(toggleTheme())}
        className="fixed right-4 top-4 z-50 rounded-md p-2 bg-[hsl(var(--card))] border border-[hsl(var(--border))] text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))] shadow-sm transition-colors"
        title={theme === "dark" ? t("common.switchLight") : t("common.switchDark")}
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
      <div className="w-full max-w-sm">
        {/* Logo */}
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
          <h1 className="text-2xl font-bold text-[hsl(var(--foreground))]">Pulse</h1>
          <p className="text-sm text-[hsl(var(--muted-foreground))]">{t("login.title")}</p>
        </div>

        {/* Card */}
        <Card>
          <form onSubmit={handleSubmit}>
            <CardHeader>
              <CardTitle>{t("login.login")}</CardTitle>
              <CardDescription>{t("login.subtitle")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {error && (
                <div className="rounded-lg border border-[hsl(var(--destructive))]/50 bg-[hsl(var(--destructive))]/10 px-4 py-2.5 text-sm text-[hsl(var(--destructive))]">
                  {error}
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="username">{t("login.username")}</Label>
                <Input
                  id="username"
                  type="text"
                  autoComplete="username"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="admin"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="password">{t("login.password")}</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                />
              </div>
            </CardContent>
            <CardFooter>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? t("login.loggingIn") : t("login.login")}
              </Button>
            </CardFooter>
          </form>
          {discourseEnabled && (
            <>
              <div className="px-6 pb-2">
                <div className="flex items-center gap-3">
                  <div className="flex-1 border-t border-[hsl(var(--border))]" />
                  <span className="text-xs text-[hsl(var(--muted-foreground))]">{t("login.or")}</span>
                  <div className="flex-1 border-t border-[hsl(var(--border))]" />
                </div>
              </div>
              <div className="px-6 pb-6">
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => { window.location.href = "/auth/discourse?spa=1"; }}
                >
                  <svg className="mr-2 h-4 w-4" viewBox="0 0 32 32" fill="currentColor">
                    <path d="M16.1357 0C7.37855 0 0 7.03 0 15.7071C0 16.26 0.0314 32 0.0314 32L16.1357 31.9373C24.8929 31.9373 32 24.6937 32 15.9687C32 7.24363 24.8929 0 16.1357 0Z" />
                  </svg>
                  {t("login.discourseLogin")}
                </Button>
              </div>
            </>
          )}
        </Card>
      </div>
    </div>
  );
}
