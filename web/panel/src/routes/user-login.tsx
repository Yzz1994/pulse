import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Card, CardHeader, CardTitle, CardContent, Button, Input } from "@/components/ui";

export default function UserLoginPage() {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit() {
    if (!username || !password || loading) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch("/v1/user/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(
          res.status === 401
            ? t("userLogin.wrongCredentials")
            : (data.error as string) || `HTTP ${res.status}`,
        );
        return;
      }
      const token = (data as { sub_token?: string }).sub_token;
      if (!token) {
        setError(t("userLogin.noToken"));
        return;
      }
      navigate({ to: "/user/$token", params: { token }, replace: true });
    } catch {
      setError(t("userLogin.networkError"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-[hsl(var(--background))] p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-center">{t("userLogin.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Input
            placeholder={t("userLogin.username")}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            autoFocus
          />
          <Input
            type="password"
            placeholder={t("userLogin.password")}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
          />
          {error && (
            <p className="text-xs text-[hsl(var(--destructive))]">{error}</p>
          )}
          <Button
            className="w-full"
            onClick={submit}
            disabled={loading || !username || !password}
          >
            {loading ? t("userLogin.loggingIn") : t("userLogin.login")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
