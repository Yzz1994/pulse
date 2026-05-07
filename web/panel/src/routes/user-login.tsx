import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Card, CardHeader, CardTitle, CardContent, Button, Input } from "@/components/ui";

export default function UserLoginPage() {
  const navigate = useNavigate();
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
            ? "用户名或密码错误"
            : (data.error as string) || `HTTP ${res.status}`,
        );
        return;
      }
      const token = (data as { sub_token?: string }).sub_token;
      if (!token) {
        setError("登录失败：服务端未返回 token");
        return;
      }
      navigate({ to: "/user/$token", params: { token }, replace: true });
    } catch {
      setError("网络错误，请重试");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-[hsl(var(--background))] p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-center">用户登录</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Input
            placeholder="用户名"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            autoFocus
          />
          <Input
            type="password"
            placeholder="密码"
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
            {loading ? "登录中…" : "登录"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
