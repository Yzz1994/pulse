import { useEffect, useState, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  Button,
  Input,
  Label,
  Separator,
  toast,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@/components/ui";
import { ScrollArea } from "@/components/ui/scroll-area";
import { api, AuthError } from "@/lib/api";
import { useAuthErrorHandler } from "@/hooks/useAuthErrorHandler";
import { clearToken, getToken } from "@/lib/auth";

// ── Types ────────────────────────────────────────────────────────

// ── Main page ────────────────────────────────────────────────────

export default function SettingsPage() {
  const navigate = useNavigate();
  const { t } = useTranslation();

  // DB stats (独立接口，避免阻塞主页加载)
  interface DBStats {
    file_size_bytes: number;
    wal_size_bytes: number;
    page_count: number;
    page_size: number;
    free_pages: number;
    ping_latency_ms: number;
    tables: Record<string, number>;
  }
  const [dbStats, setDbStats] = useState<DBStats | null>(null);
  const [dbStatsLoading, setDbStatsLoading] = useState(true);

  // Bark alert
  const [barkURL, setBarkURL] = useState("");
  const [barkLoading, setBarkLoading] = useState(true);
  const [barkSaving, setBarkSaving] = useState(false);
  const [barkTesting, setBarkTesting] = useState(false);
  const [barkMsg, setBarkMsg] = useState<string | null>(null);


  // Backup
  const [backupAccountID, setBackupAccountID] = useState("");
  const [backupAccessKeyID, setBackupAccessKeyID] = useState("");
  const [backupSecretKey, setBackupSecretKey] = useState("");
  const [backupBucketName, setBackupBucketName] = useState("");
  const [backupIntervalHours, setBackupIntervalHours] = useState("");
  const [backupKeepCount, setBackupKeepCount] = useState("");
  const [backupLastAt, setBackupLastAt] = useState("");
  const [backupLoading, setBackupLoading] = useState(true);
  const [backupSaving, setBackupSaving] = useState(false);
  const [backupRunning, setBackupRunning] = useState(false);
  const [backupMsg, setBackupMsg] = useState<string | null>(null);
  // Restore
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [restoreList, setRestoreList] = useState<{ key: string; last_modified: string; size: number }[]>([]);
  const [restoreListLoading, setRestoreListLoading] = useState(false);
  const [restoreListError, setRestoreListError] = useState<string | null>(null);
  const [restoringKey, setRestoringKey] = useState<string | null>(null);
  const [restoreMsg, setRestoreMsg] = useState<string | null>(null);

  // Stripe settings
  const [stripeKeys, setStripeKeys] = useState({
    test: { secret_key: "", webhook_secret: "" },
    live: { secret_key: "", webhook_secret: "" },
  });
  const [stripeShow, setStripeShow] = useState({
    test: { secret_key: false, webhook_secret: false },
    live: { secret_key: false, webhook_secret: false },
  });
  const [stripeLoading, setStripeLoading] = useState(true);
  const [stripeSaving, setStripeSaving] = useState(false);
  const [stripeMsg, setStripeMsg] = useState<string | null>(null);

  // Shop settings
  const [shopBaseURL, setShopBaseURL] = useState("");
  const [shopLoading, setShopLoading] = useState(true);
  const [shopSaving, setShopSaving] = useState(false);
  const [shopMsg, setShopMsg] = useState<string | null>(null);

  // Cloudflare API Token
  const [cfToken, setCfToken] = useState("");
  const [cfHasToken, setCfHasToken] = useState(false);
  const [cfSaving, setCfSaving] = useState(false);
  const [cfShowToken, setCfShowToken] = useState(false);

  // DB Cleanup
  const [dbCleaning, setDbCleaning] = useState(false);

  // Log retention settings
  const [logUptimeDays, setLogUptimeDays] = useState(30);
  const [logDailyDays, setLogDailyDays] = useState(180);
  const [logRetentionLoading, setLogRetentionLoading] = useState(true);
  const [logRetentionSaving, setLogRetentionSaving] = useState(false);
  const [logCleaning, setLogCleaning] = useState(false);

  // Nodes Apply
  const [nodesApplying, setNodesApplying] = useState(false);

  // MaxMind
  const [maxmindKey, setMaxmindKey] = useState("");
  const [maxmindHasKey, setMaxmindHasKey] = useState(false);
  const [maxmindDBReady, setMaxmindDBReady] = useState(false);
  const [maxmindSaving, setMaxmindSaving] = useState(false);
  const [maxmindDownloading, setMaxmindDownloading] = useState(false);
  const [maxmindShowKey, setMaxmindShowKey] = useState(false);

  // 账户安全 — 修改密码
  const [credUsername, setCredUsername] = useState("");
  const [credOldPassword, setCredOldPassword] = useState("");
  const [credNewPassword, setCredNewPassword] = useState("");
  const [credConfirmPassword, setCredConfirmPassword] = useState("");
  const [credSaving, setCredSaving] = useState(false);
  const [credMsg, setCredMsg] = useState<string | null>(null);

  // ── Auth redirect helper ─────────────────────────────────────

  const handleAuthError = useAuthErrorHandler();

  const fetchDbStats = useCallback(() => {
    setDbStatsLoading(true);
    api
      .get<DBStats>("/system/db/stats")
      .then(setDbStats)
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setDbStatsLoading(false));
  }, [handleAuthError]);

  // ── Fetch certificate (removed in gRPC push model) ───────────

  useEffect(() => {
    fetchDbStats();
    fetchStripeSettings();
    fetchShopSettings();
    fetchBarkAlert();
    fetchBackupSettings();
    fetchCFToken();
    fetchMaxmindStatus();
    api.get<{ uptime_retain_days: number; daily_retain_days: number }>("/settings/logs")
      .then((d) => { setLogUptimeDays(d.uptime_retain_days); setLogDailyDays(d.daily_retain_days); })
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setLogRetentionLoading(false));
  }, [fetchDbStats]);

  function fetchCFToken() {
    api
      .get<{ has_token: boolean; token: string }>("/settings/cf-token")
      .then((d) => { setCfHasToken(d.has_token); setCfToken(d.token ?? ""); })
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  function saveCFToken() {
    setCfSaving(true);
    api
      .put<{ ok: boolean }>("/settings/cf-token", { token: cfToken })
      .then(() => {
        setCfHasToken(cfToken !== "");
        toast(t("settings.cfTokenSaved"), "success");
      })
      .catch((err) => {
        if (handleAuthError(err)) return;
        toast(err instanceof Error ? err.message : t("common.saveFailed"), "error");
      })
      .finally(() => setCfSaving(false));
  }

  // ── MaxMind ───────────────────────────────────────────────────

  function fetchMaxmindStatus() {
    api
      .get<{ has_key: boolean; key: string; db_ready: boolean }>("/settings/maxmind")
      .then((d) => { setMaxmindHasKey(d.has_key); setMaxmindKey(d.key ?? ""); setMaxmindDBReady(d.db_ready); })
      .catch((err) => { if (handleAuthError(err)) return; });
  }

  function saveMaxmindKey() {
    setMaxmindSaving(true);
    api
      .put<{ ok: boolean }>("/settings/maxmind", { key: maxmindKey })
      .then(() => {
        setMaxmindHasKey(maxmindKey !== "");
        toast(t("settings.maxmindKeySaved"), "success");
        downloadMaxmindDB();
      })
      .catch((err) => { if (handleAuthError(err)) return; toast(err instanceof Error ? err.message : t("common.saveFailed"), "error"); })
      .finally(() => setMaxmindSaving(false));
  }

  function downloadMaxmindDB() {
    setMaxmindDownloading(true);
    api
      .post<{ ok: boolean }>("/system/geoip/download", {})
      .then(() => {
        toast(t("settings.dbDownloadStarted"), "success");
        // 轮询 db_ready
        const poll = setInterval(() => {
          api.get<{ has_key: boolean; db_ready: boolean }>("/settings/maxmind")
            .then((d) => {
              if (d.db_ready) {
                setMaxmindDBReady(true);
                setMaxmindDownloading(false);
                toast(t("settings.geoipReady"), "success");
                clearInterval(poll);
              }
            })
            .catch(() => {});
        }, 3000);
        setTimeout(() => { clearInterval(poll); setMaxmindDownloading(false); }, 120000);
      })
      .catch((err) => { if (handleAuthError(err)) return; toast(err instanceof Error ? err.message : t("settings.downloadFailed"), "error"); setMaxmindDownloading(false); });
  }

  // ── Stripe settings ───────────────────────────────────────────

  function fetchStripeSettings() {
    setStripeLoading(true);
    api
      .get<{
        mode: "test" | "live";
        test: { secret_key: string; webhook_secret: string };
        live: { secret_key: string; webhook_secret: string };
      }>("/settings/stripe")
      .then((d) => {
        setStripeKeys({ test: d.test, live: d.live });
      })
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setStripeLoading(false));
  }

  function saveStripeSettings() {
    setStripeSaving(true);
    setStripeMsg(null);
    api
      .put<{ saved: boolean }>("/settings/stripe", {
        test_secret_key: stripeKeys.test.secret_key,
        test_webhook_secret: stripeKeys.test.webhook_secret,
        live_secret_key: stripeKeys.live.secret_key,
        live_webhook_secret: stripeKeys.live.webhook_secret,
      })
      .then(() => setStripeMsg(t("settings.savedAndActive")))
      .catch((err) => { if (handleAuthError(err)) return; setStripeMsg(t("common.saveFailed")); })
      .finally(() => { setStripeSaving(false); setTimeout(() => setStripeMsg(null), 3000); });
  }

  // ── Shop settings ─────────────────────────────────────────────

  function fetchShopSettings() {
    setShopLoading(true);
    api
      .get<{ base_url: string }>("/settings/shop")
      .then((d) => setShopBaseURL(d.base_url ?? ""))
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setShopLoading(false));
  }

  function saveShopSettings() {
    setShopSaving(true);
    setShopMsg(null);
    api
      .put<{ saved: boolean }>("/settings/shop", { base_url: shopBaseURL })
      .then(() => setShopMsg(t("common.saved")))
      .catch((err) => { if (handleAuthError(err)) return; setShopMsg(t("common.saveFailed")); })
      .finally(() => { setShopSaving(false); setTimeout(() => setShopMsg(null), 2500); });
  }

  // ── Fetch Bark alert settings ──────────────────────────────────

  function fetchBarkAlert() {
    setBarkLoading(true);
    api
      .get<{ bark_url: string }>("/settings/alert")
      .then((data) => {
        setBarkURL(data.bark_url);
      })
      .catch((err) => {
        if (handleAuthError(err)) return;
      })
      .finally(() => setBarkLoading(false));
  }

  function saveBarkAlert() {
    setBarkSaving(true);
    setBarkMsg(null);
    api
      .put<{ saved: boolean }>("/settings/alert", { bark_url: barkURL })
      .then(() => setBarkMsg(t("common.saved")))
      .catch((err) => {
        if (handleAuthError(err)) return;
        setBarkMsg(err instanceof Error ? err.message : t("common.saveFailed"));
      })
      .finally(() => {
        setBarkSaving(false);
        setTimeout(() => setBarkMsg(null), 2500);
      });
  }

  function testBarkAlert() {
    setBarkTesting(true);
    setBarkMsg(null);
    api
      .post<{ sent?: boolean; error?: string }>("/settings/alert/test", {})
      .then((data) => {
        if (data.error) {
          setBarkMsg(data.error);
        } else {
          setBarkMsg(t("settings.pushSuccess"));
        }
      })
      .catch((err) => {
        if (handleAuthError(err)) return;
        setBarkMsg(err instanceof Error ? err.message : t("settings.pushFailed"));
      })
      .finally(() => {
        setBarkTesting(false);
        setTimeout(() => setBarkMsg(null), 2500);
      });
  }

  // ── Backup settings ──────────────────────────────────────────

  function fetchBackupSettings() {
    setBackupLoading(true);
    api
      .get<{ account_id: string; access_key_id: string; bucket_name: string; interval_hours: string; keep_count: string; last_at: string }>("/settings/backup")
      .then((d) => {
        setBackupAccountID(d.account_id ?? "");
        setBackupAccessKeyID(d.access_key_id ?? "");
        setBackupBucketName(d.bucket_name ?? "");
        setBackupIntervalHours(d.interval_hours ?? "");
        setBackupKeepCount(d.keep_count ?? "");
        setBackupLastAt(d.last_at ?? "");
      })
      .catch((err) => { if (handleAuthError(err)) return; })
      .finally(() => setBackupLoading(false));
  }

  function saveBackupSettings() {
    setBackupSaving(true);
    setBackupMsg(null);
    const body: Record<string, string> = {
      account_id: backupAccountID,
      access_key_id: backupAccessKeyID,
      bucket_name: backupBucketName,
      interval_hours: backupIntervalHours,
      keep_count: backupKeepCount,
    };
    if (backupSecretKey) body.secret_key = backupSecretKey;
    api
      .put<{ saved: boolean }>("/settings/backup", body)
      .then(() => { setBackupMsg(t("common.saved")); setBackupSecretKey(""); })
      .catch((err) => { if (handleAuthError(err)) return; setBackupMsg(t("common.saveFailed")); })
      .finally(() => { setBackupSaving(false); setTimeout(() => setBackupMsg(null), 2500); });
  }

  function runBackup() {
    setBackupRunning(true);
    setBackupMsg(null);
    api
      .post<{ ok: boolean; last_at: string }>("/settings/backup/run", {})
      .then((d) => { setBackupLastAt(d.last_at ?? ""); setBackupMsg(t("settings.backupSuccess")); })
      .catch((err) => { if (handleAuthError(err)) return; setBackupMsg(t("settings.backupFailed")); })
      .finally(() => { setBackupRunning(false); setTimeout(() => setBackupMsg(null), 3000); });
  }

  function openRestoreDialog() {
    setRestoreOpen(true);
    setRestoreMsg(null);
    setRestoreListError(null);
    setRestoreListLoading(true);
    api
      .get<{ backups: { key: string; last_modified: string; size: number }[] }>("/settings/backup/list")
      .then((d) => { setRestoreList(d.backups ?? []); })
      .catch((err) => { if (handleAuthError(err)) return; setRestoreListError(t("settings.loadBackupListFailed")); })
      .finally(() => { setRestoreListLoading(false); });
  }

  function doRestore(key: string) {
    setRestoringKey(key);
    setRestoreMsg(null);
    api
      .post<{ ok: boolean }>("/settings/backup/restore", { key })
      .then(() => { setRestoreMsg(t("settings.restoreReady")); })
      .catch((err) => { if (handleAuthError(err)) return; setRestoreMsg(t("settings.restoreFailed")); })
      .finally(() => { setRestoringKey(null); });
  }

  // ── 修改管理员凭据 ───────────────────────────────────────────────

  async function saveCredentials() {
    setCredMsg(null);
    if (!credOldPassword || !credNewPassword) {
      setCredMsg(t("settings.oldNewPasswordRequired"));
      return;
    }
    if (credNewPassword !== credConfirmPassword) {
      setCredMsg(t("settings.passwordMismatch"));
      return;
    }
    if (credNewPassword.length < 6) {
      setCredMsg(t("settings.passwordMinLength"));
      return;
    }
    setCredSaving(true);
    try {
      const body: Record<string, string> = {
        old_password: credOldPassword,
        new_password: credNewPassword,
      };
      if (credUsername.trim()) body.username = credUsername.trim();
      await api.put("/auth/credentials", body);
      toast(t("settings.passwordUpdated"), "success");
      setCredOldPassword("");
      setCredNewPassword("");
      setCredConfirmPassword("");
      setCredUsername("");
      setCredMsg(t("common.saved"));
    } catch (err) {
      if (handleAuthError(err as Error)) return;
      setCredMsg(err instanceof Error ? err.message : t("common.saveFailed"));
    } finally {
      setCredSaving(false);
    }
  }

  // ── Render ───────────────────────────────────────────────────

  return (
    <div className="p-4 sm:p-6 lg:p-8">
      <h1 className="mb-6 text-2xl font-bold text-[hsl(var(--foreground))]">
        {t("settings.title")}
      </h1>

      <Tabs defaultValue="general" className="w-full">
        <TabsList className="mb-6">
          <TabsTrigger value="general">{t("settings.general")}</TabsTrigger>
          <TabsTrigger value="integrations">{t("settings.integrations")}</TabsTrigger>
          <TabsTrigger value="database">{t("settings.database")}</TabsTrigger>
        </TabsList>

        {/* ── 常规 Tab ──────────────────────────────────────────── */}
        <TabsContent value="general" className="grid gap-6 mt-0">
          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.alertPush")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4">
              {barkLoading ? (
                <div className="space-y-3">
                  <div className="h-5 w-24 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="bark-url" className="text-sm">
                      {t("settings.barkURL")}
                    </Label>
                    <Input
                      id="bark-url"
                      value={barkURL}
                      onChange={(e) => setBarkURL(e.target.value)}
                      placeholder="https://api.day.app/your-device-key"
                    />
                  </div>

                  <div className="flex items-center gap-3">
                    <Button
                      size="sm"
                      onClick={saveBarkAlert}
                      disabled={barkSaving}
                    >
                      {barkSaving ? t("common.saving") : t("settings.saveSettings")}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={testBarkAlert}
                      disabled={barkTesting}
                    >
                      {barkTesting ? t("settings.pushing") : t("settings.testPush")}
                    </Button>
                    {barkMsg && (
                      <span className="text-sm text-[hsl(var(--muted-foreground))]">
                        {barkMsg}
                      </span>
                    )}
                  </div>

                  <p className="text-xs text-[hsl(var(--muted-foreground))]">
                    {t("settings.barkDesc")}
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          {/* 账户安全 */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.accountSecurity")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4 space-y-4">
              <p className="text-sm text-[hsl(var(--muted-foreground))]">
                {t("settings.accountSecurityDesc")}
              </p>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label className="text-sm">{t("settings.newUsername")}</Label>
                  <Input
                    type="text"
                    autoComplete="username"
                    placeholder={t("settings.leaveEmptyNoChange")}
                    value={credUsername}
                    onChange={(e) => setCredUsername(e.target.value)}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-sm">{t("settings.oldPassword")}</Label>
                  <Input
                    type="password"
                    autoComplete="current-password"
                    placeholder="••••••••"
                    value={credOldPassword}
                    onChange={(e) => setCredOldPassword(e.target.value)}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-sm">{t("settings.newPassword")}</Label>
                  <Input
                    type="password"
                    autoComplete="new-password"
                    placeholder="••••••••"
                    value={credNewPassword}
                    onChange={(e) => setCredNewPassword(e.target.value)}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-sm">{t("settings.confirmPassword")}</Label>
                  <Input
                    type="password"
                    autoComplete="new-password"
                    placeholder="••••••••"
                    value={credConfirmPassword}
                    onChange={(e) => setCredConfirmPassword(e.target.value)}
                  />
                </div>
              </div>
              <div className="flex items-center gap-3">
                <Button size="sm" onClick={saveCredentials} disabled={credSaving}>
                  {credSaving ? t("common.saving") : t("common.save")}
                </Button>
                {credMsg && (
                  <span className="text-sm text-[hsl(var(--muted-foreground))]">{credMsg}</span>
                )}
              </div>
            </CardContent>
          </Card>

        </TabsContent>

        {/* ── 集成 Tab ──────────────────────────────────────────── */}
        <TabsContent value="integrations" className="grid gap-6 mt-0">
          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.maxmindTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-[hsl(var(--muted-foreground))]">
                {t("settings.maxmindDesc")}
                {maxmindDBReady && <span className="ml-1 text-green-600">{t("settings.dbReady")}</span>}
                {maxmindHasKey && !maxmindDBReady && maxmindDownloading && <span className="ml-1 text-amber-500">{t("settings.dbDownloading")}</span>}
                {maxmindHasKey && !maxmindDBReady && !maxmindDownloading && <span className="ml-1 text-amber-500">{t("settings.dbNotReady")}</span>}
              </p>
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Input
                    type={maxmindShowKey ? "text" : "password"}
                    placeholder="MaxMind License Key"
                    value={maxmindKey}
                    onChange={(e) => setMaxmindKey(e.target.value)}
                    className="font-mono text-sm pr-9"
                  />
                  <button
                    type="button"
                    onClick={() => setMaxmindShowKey((v) => !v)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                    tabIndex={-1}
                  >
                    {maxmindShowKey ? (
                      <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
                      </svg>
                    ) : (
                      <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                      </svg>
                    )}
                  </button>
                </div>
                <Button size="sm" onClick={saveMaxmindKey} disabled={maxmindSaving}>
                  {maxmindSaving ? t("common.saving") : t("common.save")}
                </Button>
              </div>
              {maxmindHasKey && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={downloadMaxmindDB}
                  disabled={maxmindDownloading}
                >
                  {maxmindDownloading ? t("settings.downloading") : maxmindDBReady ? t("settings.updateDB") : t("settings.downloadDB")}
                </Button>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.cfTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-[hsl(var(--muted-foreground))]">
                {t("settings.cfDesc")}
                {cfHasToken && <span className="ml-1 text-green-600">{t("settings.cfConfigured")}</span>}
              </p>
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Input
                    type={cfShowToken ? "text" : "password"}
                    placeholder="CF API Token"
                    value={cfToken}
                    onChange={(e) => setCfToken(e.target.value)}
                    className="font-mono text-sm pr-9"
                  />
                  <button
                    type="button"
                    onClick={() => setCfShowToken((v) => !v)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                    tabIndex={-1}
                  >
                    {cfShowToken ? (
                      <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
                      </svg>
                    ) : (
                      <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                      </svg>
                    )}
                  </button>
                </div>
                <Button size="sm" onClick={saveCFToken} disabled={cfSaving}>
                  {cfSaving ? t("common.saving") : t("common.save")}
                </Button>
              </div>
              <p className="text-xs text-[hsl(var(--muted-foreground))]">
                {t("settings.cfSaveHint")}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.stripeTitle")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4 space-y-4">
              {stripeLoading ? (
                <div className="space-y-3">
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                </div>
              ) : (
                <>
                  <p className="text-sm text-[hsl(var(--muted-foreground))]">
                    {t("settings.stripeDesc")}
                  </p>

                  {/* 两套密钥，直接展开 */}
                  {(["live", "test"] as const).map((env) => (
                    <div key={env} className="space-y-3">
                      <p className="text-sm font-medium text-[hsl(var(--foreground))]">
                        {env === "live" ? t("settings.stripeLiveKeys") : t("settings.stripeTestKeys")}
                      </p>
                      {(["secret_key", "webhook_secret"] as const).map((field) => {
                        const label = field === "secret_key" ? "Secret Key" : "Webhook Secret";
                        const placeholder = field === "secret_key"
                          ? env === "test" ? "sk_test_..." : "sk_live_..."
                          : "whsec_...";
                        const visible = stripeShow[env][field];
                        return (
                          <div key={field} className="space-y-1.5">
                            <Label className="text-sm">{label}</Label>
                            <div className="relative">
                              <Input
                                type={visible ? "text" : "password"}
                                value={stripeKeys[env][field]}
                                onChange={(e) =>
                                  setStripeKeys((prev) => ({
                                    ...prev,
                                    [env]: { ...prev[env], [field]: e.target.value },
                                  }))
                                }
                                placeholder={placeholder}
                                className="font-mono text-sm pr-10"
                              />
                              <button
                                type="button"
                                onClick={() =>
                                  setStripeShow((prev) => ({
                                    ...prev,
                                    [env]: { ...prev[env], [field]: !prev[env][field] },
                                  }))
                                }
                                className="absolute right-2 top-1/2 -translate-y-1/2 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]"
                                tabIndex={-1}
                              >
                                {visible ? (
                                  <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
                                  </svg>
                                ) : (
                                  <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                                  </svg>
                                )}
                              </button>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  ))}

                  <div className="flex items-center gap-3">
                    <Button size="sm" onClick={saveStripeSettings} disabled={stripeSaving}>
                      {stripeSaving ? t("common.saving") : t("common.save")}
                    </Button>
                    {stripeMsg && (
                      <span className="text-sm text-[hsl(var(--muted-foreground))]">{stripeMsg}</span>
                    )}
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.shopTitle")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4">
              {shopLoading ? (
                <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
              ) : (
                <div className="space-y-4">
                  <p className="text-sm text-[hsl(var(--muted-foreground))]">
                    {t("settings.shopDesc")}
                  </p>
                  <div className="space-y-2">
                    <Label className="text-sm">{t("settings.shopBaseURL")}</Label>
                    <Input
                      value={shopBaseURL}
                      onChange={(e) => setShopBaseURL(e.target.value)}
                      placeholder="https://example.com"
                      className="font-mono text-sm"
                    />
                  </div>
                  <div className="flex items-center gap-3">
                    <Button size="sm" onClick={saveShopSettings} disabled={shopSaving}>
                      {shopSaving ? t("common.saving") : t("common.save")}
                    </Button>
                    {shopMsg && (
                      <span className="text-sm text-[hsl(var(--muted-foreground))]">{shopMsg}</span>
                    )}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* ── 数据库 Tab ────────────────────────────────────────── */}
        <TabsContent value="database" className="grid gap-6 mt-0">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle className="text-base font-medium">{t("settings.dbStatus")}</CardTitle>
              <Button
                variant="ghost"
                size="sm"
                onClick={fetchDbStats}
                disabled={dbStatsLoading}
                className="text-xs text-[hsl(var(--muted-foreground))]"
              >
                {dbStatsLoading ? t("common.loading") : t("common.refresh")}
              </Button>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4 space-y-4">
              {dbStatsLoading ? (
                <div className="space-y-2">
                  <div className="h-4 w-3/4 animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-4 w-1/2 animate-pulse rounded bg-[hsl(var(--muted))]" />
                </div>
              ) : dbStats ? (
                <>
                  <div className="grid grid-cols-2 gap-x-8 gap-y-1.5 text-sm sm:grid-cols-3">
                    <div className="flex justify-between gap-2">
                      <span className="text-[hsl(var(--muted-foreground))]">{t("settings.dbSize")}</span>
                      <span className="font-mono">{dbStats.file_size_bytes >= 1024 * 1024 ? `${(dbStats.file_size_bytes / 1024 / 1024).toFixed(1)} MB` : `${(dbStats.file_size_bytes / 1024).toFixed(1)} KB`}</span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-[hsl(var(--muted-foreground))]">{t("settings.pingLatency")}</span>
                      <span className="font-mono">{dbStats.ping_latency_ms.toFixed(2)} ms</span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-[hsl(var(--muted-foreground))]">{t("settings.tableCount")}</span>
                      <span className="font-mono">{Object.keys(dbStats.tables).length}</span>
                    </div>
                  </div>
                  {Object.keys(dbStats.tables).length > 0 && (
                    <div className="rounded-md border border-[hsl(var(--border))] overflow-hidden">
                      <div className="grid grid-cols-2 divide-x divide-[hsl(var(--border))] sm:grid-cols-3 lg:grid-cols-4">
                        {Object.entries(dbStats.tables)
                          .sort((a, b) => b[1] - a[1])
                          .map(([tbl, count]) => (
                            <div key={tbl} className="flex items-center justify-between px-3 py-1.5 text-xs border-b border-[hsl(var(--border))]">
                              <span className="text-[hsl(var(--muted-foreground))] truncate mr-2">{tbl}</span>
                              <span className="font-mono shrink-0">{count.toLocaleString()}</span>
                            </div>
                          ))}
                      </div>
                    </div>
                  )}
                </>
              ) : (
                <p className="text-sm text-[hsl(var(--muted-foreground))]">{t("settings.loadFailed")}</p>
              )}
              <div className="flex items-center gap-3 pt-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={dbCleaning}
                  onClick={() => {
                    setDbCleaning(true);
                    api.post<{ total: number }>("/system/db/cleanup", {})
                      .then((r) => {
                        toast(r.total > 0 ? t("settings.cleanedCount", { count: r.total }) : t("settings.dataConsistent"), "success");
                        fetchDbStats();
                      })
                      .catch((err) => {
                        if (handleAuthError(err)) return;
                        toast(err instanceof Error ? err.message : t("settings.cleanupFailed"), "error");
                      })
                      .finally(() => setDbCleaning(false));
                  }}
                >
                  {dbCleaning ? t("common.saving") : t("settings.cleanupRedundant")}
                </Button>
                <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("settings.cleanupRedundantDesc")}</p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-base font-medium">{t("settings.nodeApplyTitle")}</CardTitle>
                <p className="text-sm text-[hsl(var(--muted-foreground))] mt-1">{t("settings.nodeApplyDesc")}</p>
              </div>
              <Button
                variant="outline"
                size="sm"
                disabled={nodesApplying}
                onClick={() => {
                  setNodesApplying(true);
                  api.post<{ nodes: number }>("/system/nodes/apply", {})
                    .then((r) => {
                      toast(t("settings.nodesApplied", { count: r.nodes }), "success");
                    })
                    .catch((err) => {
                      if (handleAuthError(err)) return;
                      toast(err instanceof Error ? err.message : t("settings.applyFailed"), "error");
                    })
                    .finally(() => setNodesApplying(false));
                }}
              >
                {nodesApplying ? t("settings.applying") : t("settings.reapplyConfig")}
              </Button>
            </CardHeader>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.logRetention")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4 space-y-4">
              {logRetentionLoading ? (
                <div className="space-y-3">
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                </div>
              ) : (
                <>
                  <p className="text-sm text-[hsl(var(--muted-foreground))]">
                    {t("settings.logRetentionDesc")}
                  </p>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-1.5">
                      <Label className="text-sm">{t("settings.uptimeLogRetention")}</Label>
                      <Input
                        type="number"
                        min={1}
                        value={logUptimeDays}
                        onChange={(e) => setLogUptimeDays(Number(e.target.value) || 30)}
                        className="font-mono"
                      />
                      <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("settings.defaultDays", { days: 30 })}</p>
                    </div>
                    <div className="space-y-1.5">
                      <Label className="text-sm">{t("settings.dailyLogRetention")}</Label>
                      <Input
                        type="number"
                        min={1}
                        value={logDailyDays}
                        onChange={(e) => setLogDailyDays(Number(e.target.value) || 180)}
                        className="font-mono"
                      />
                      <p className="text-xs text-[hsl(var(--muted-foreground))]">{t("settings.defaultDays", { days: 180 })}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <Button
                      size="sm"
                      disabled={logRetentionSaving}
                      onClick={() => {
                        setLogRetentionSaving(true);
                        api.put("/settings/logs", { uptime_retain_days: logUptimeDays, daily_retain_days: logDailyDays })
                          .then(() => toast(t("settings.retentionPolicySaved"), "success"))
                          .catch((err) => { if (handleAuthError(err)) return; toast(t("common.saveFailed"), "error"); })
                          .finally(() => setLogRetentionSaving(false));
                      }}
                    >
                      {logRetentionSaving ? t("common.saving") : t("common.save")}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={logCleaning}
                      onClick={() => {
                        setLogCleaning(true);
                        api.post<{ uptime_retain_days: number; daily_retain_days: number }>("/system/logs/cleanup", {})
                          .then((r) => toast(t("settings.logCleaned", { uptime: r.uptime_retain_days, daily: r.daily_retain_days }), "success"))
                          .catch((err) => { if (handleAuthError(err)) return; toast(t("settings.cleanupFailed"), "error"); })
                          .finally(() => setLogCleaning(false));
                      }}
                    >
                      {logCleaning ? t("common.saving") : t("settings.cleanupNow")}
                    </Button>
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base font-medium">{t("settings.backupTitle")}</CardTitle>
            </CardHeader>
            <Separator />
            <CardContent className="pt-4">
              {backupLoading ? (
                <div className="space-y-3">
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                  <div className="h-9 w-full animate-pulse rounded bg-[hsl(var(--muted))]" />
                </div>
              ) : (
                <div className="space-y-4">
                  <p className="text-sm text-[hsl(var(--muted-foreground))]">
                    {t("settings.backupDesc")}
                  </p>
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div className="space-y-2">
                      <Label className="text-sm">Account ID</Label>
                      <Input value={backupAccountID} onChange={(e) => setBackupAccountID(e.target.value)} placeholder="Cloudflare Account ID" className="font-mono text-sm" />
                    </div>
                    <div className="space-y-2">
                      <Label className="text-sm">{t("settings.bucketName")}</Label>
                      <Input value={backupBucketName} onChange={(e) => setBackupBucketName(e.target.value)} placeholder="my-pulse-backup" className="font-mono text-sm" />
                    </div>
                    <div className="space-y-2">
                      <Label className="text-sm">Access Key ID</Label>
                      <Input value={backupAccessKeyID} onChange={(e) => setBackupAccessKeyID(e.target.value)} placeholder="R2 Access Key ID" className="font-mono text-sm" />
                    </div>
                    <div className="space-y-2">
                      <Label className="text-sm">Secret Access Key</Label>
                      <Input
                        type="password"
                        value={backupSecretKey}
                        onChange={(e) => setBackupSecretKey(e.target.value)}
                        placeholder={backupAccountID ? t("settings.setKeepEmpty") : "R2 Secret Key"}
                        className="font-mono text-sm"
                        autoComplete="new-password"
                      />
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-4">
                    <div className="space-y-2 w-44">
                      <Label className="text-sm">{t("settings.backupInterval")}</Label>
                      <Input
                        type="number"
                        value={backupIntervalHours}
                        onChange={(e) => setBackupIntervalHours(e.target.value)}
                        placeholder={t("settings.disabledZero")}
                        min={0}
                        max={720}
                      />
                    </div>
                    <div className="space-y-2 w-44">
                      <Label className="text-sm">{t("settings.keepCount")}</Label>
                      <Input
                        type="number"
                        value={backupKeepCount}
                        onChange={(e) => setBackupKeepCount(e.target.value)}
                        placeholder={t("settings.noLimitZero")}
                        min={0}
                        max={365}
                      />
                    </div>
                  </div>
                  {backupLastAt && (
                    <p className="text-xs text-[hsl(var(--muted-foreground))]">
                      {t("settings.lastBackup", { time: new Date(backupLastAt).toLocaleString() })}
                    </p>
                  )}
                  <div className="flex items-center gap-3">
                    <Button size="sm" onClick={saveBackupSettings} disabled={backupSaving}>
                      {backupSaving ? t("common.saving") : t("common.save")}
                    </Button>
                    <Button size="sm" variant="outline" onClick={runBackup} disabled={backupRunning}>
                      {backupRunning ? t("settings.backuping") : t("settings.backupNow")}
                    </Button>
                    <Button size="sm" variant="outline" onClick={openRestoreDialog} disabled={!backupAccountID}>
                       {t("settings.restoreFromBackup")}
                    </Button>
                    {backupMsg && <span className="text-sm text-[hsl(var(--muted-foreground))]">{backupMsg}</span>}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Restore dialog */}
          {restoreOpen && (
            <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={() => setRestoreOpen(false)}>
              <div className="w-full max-w-lg rounded-lg border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-6 shadow-xl" onClick={(e) => e.stopPropagation()}>
                <div className="mb-4 flex items-center justify-between">
                  <h2 className="text-base font-semibold">{t("settings.restoreTitle")}</h2>
                  <button onClick={() => setRestoreOpen(false)} className="text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]">✕</button>
                </div>
                {restoreMsg ? (
                  <div className="space-y-4">
                    <p className="text-sm text-[hsl(var(--muted-foreground))]">{restoreMsg}</p>
                    <Button size="sm" onClick={() => setRestoreOpen(false)}>{t("common.close")}</Button>
                  </div>
                ) : restoreListLoading ? (
                  <div className="space-y-2">
                    {[1,2,3].map(i => <div key={i} className="h-10 animate-pulse rounded bg-[hsl(var(--muted))]" />)}
                  </div>
                ) : restoreListError ? (
                  <p className="text-sm text-[hsl(var(--destructive))]">{restoreListError}</p>
                ) : restoreList.length === 0 ? (
                  <p className="text-sm text-[hsl(var(--muted-foreground))]">{t("settings.noBackupFiles")}</p>
                ) : (
                  <ScrollArea className="max-h-80">
                    <div className="space-y-1">
                    {restoreList.map((b) => (
                      <div key={b.key} className="flex items-center justify-between rounded-md border border-[hsl(var(--border))] px-3 py-2">
                        <div>
                          <p className="text-sm font-mono">{b.key}</p>
                          <p className="text-xs text-[hsl(var(--muted-foreground))]">
                            {new Date(b.last_modified).toLocaleString("zh-CN")} · {(b.size / 1024 / 1024).toFixed(1)} MB
                          </p>
                        </div>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => doRestore(b.key)}
                          disabled={restoringKey !== null}
                        >
                          {restoringKey === b.key ? t("settings.restoring") : t("settings.restore")}
                        </Button>
                      </div>
                    ))}
                    </div>
                  </ScrollArea>
                )}
              </div>
            </div>
          )}

        </TabsContent>
      </Tabs>
    </div>
  );
}
