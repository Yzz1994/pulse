import {
  createRouter,
  createRoute,
  createRootRoute,
  redirect,
  Navigate,
} from "@tanstack/react-router";
import { isAuthenticated } from "./lib/auth";
import RootLayout from "./routes/root-layout";
import LoginPage from "./routes/login";
import SetupPage from "./routes/setup";
import DashboardPage from "./routes/dashboard";
import UsersPage from "./routes/users";
import NodesPage from "./routes/nodes";
import InboundsPage from "./routes/inbounds";
import OutboundsPage from "./routes/outbounds";
import SettingsPage from "./routes/settings";
import RouteRulesPage from "./routes/routerules";
import SNIProxyPage from "./routes/sniproxy";
import ShopPage from "./routes/shop";
import ShopSuccessPage from "./routes/shop-success";
import UserPage from "./routes/user";
import PlansPage from "./routes/plans";
import StatPage from "./routes/stat";
import AnnouncementsPage from "./routes/announcements";
import TicketsPage from "./routes/tickets";
import DomainsPage from "./routes/domains";
import UserGroupsPage from "./routes/user-groups";
import TraceRoutePage from "./routes/traceroute";
import LatencyPage from "./routes/latency";
import AuditPage from "./routes/audit";

/* ── Auth guard ───────────────────────────────────────────────── */

function requireAuth() {
  if (!isAuthenticated()) {
    throw redirect({ to: "/panel/login" });
  }
}

/* ── Setup status helper ──────────────────────────────────────── */

async function fetchSetupStatus(): Promise<boolean> {
  try {
    const res = await fetch("/v1/auth/setup-status");
    const data = await res.json();
    return Boolean(data.needs_setup);
  } catch {
    return false;
  }
}

/* ── Root ─────────────────────────────────────────────────────── */

const rootRoute = createRootRoute({
  component: RootLayout,
  notFoundComponent: () => (
    <Navigate to={isAuthenticated() ? "/panel/dashboard" : "/panel/login"} />
  ),
});

/* ── Setup (standalone, no sidebar) ───────────────────────────── */

const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/setup",
  component: SetupPage,
  beforeLoad: async () => {
    const needsSetup = await fetchSetupStatus();
    // 若已有管理员，不需要 setup，跳转到 login
    if (!needsSetup) {
      throw redirect({ to: "/panel/login" });
    }
  },
});

/* ── Login (standalone, no sidebar) ───────────────────────────── */

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/login",
  component: LoginPage,
  beforeLoad: async () => {
    // 若已登录，直接进 dashboard
    if (isAuthenticated()) {
      throw redirect({ to: "/panel/dashboard" });
    }
    // 若尚未初始化，跳转到 setup
    const needsSetup = await fetchSetupStatus();
    if (needsSetup) {
      throw redirect({ to: "/panel/setup" });
    }
  },
});

/* ── Index redirect ───────────────────────────────────────────── */

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/panel/dashboard" });
  },
});

const panelIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel",
  beforeLoad: () => {
    throw redirect({ to: "/panel/dashboard" });
  },
});

/* ── App pages (rendered inside the sidebar layout) ───────────── */

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/dashboard",
  component: DashboardPage,
  beforeLoad: requireAuth,
});

const usersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/users",
  component: UsersPage,
  beforeLoad: requireAuth,
});

const nodesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/nodes",
  component: NodesPage,
  beforeLoad: requireAuth,
});

const inboundsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/inbounds",
  component: InboundsPage,
  beforeLoad: requireAuth,
});

const outboundsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/outbounds",
  component: OutboundsPage,
  beforeLoad: requireAuth,
});

const routerulesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/routerules",
  component: RouteRulesPage,
  beforeLoad: requireAuth,
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/settings",
  component: SettingsPage,
  beforeLoad: requireAuth,
});

const sniproxyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/sniproxy",
  component: SNIProxyPage,
  beforeLoad: requireAuth,
});

const plansRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/plans",
  component: PlansPage,
  beforeLoad: requireAuth,
});

const announcementsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/announcements",
  component: AnnouncementsPage,
  beforeLoad: requireAuth,
});

const ticketsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/tickets",
  component: TicketsPage,
  beforeLoad: requireAuth,
});

const domainsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/domains",
  component: DomainsPage,
  beforeLoad: requireAuth,
});

const userGroupsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/user-groups",
  component: UserGroupsPage,
  beforeLoad: requireAuth,
});

const tracerouteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/traceroute",
  component: TraceRoutePage,
  beforeLoad: requireAuth,
});

const latencyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/latency",
  component: LatencyPage,
  beforeLoad: requireAuth,
});

const auditRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/panel/audit",
  component: AuditPage,
  beforeLoad: requireAuth,
});

/* ── Public pages (no auth required) ──────────────────────────── */

const shopRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/shop",
  component: ShopPage,
});

const shopSuccessRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/shop/success",
  component: ShopSuccessPage,
});

const shopTestRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/shop-test",
  component: () => <ShopPage basePath="/shop-test" />,
});

const shopTestSuccessRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/shop-test/success",
  component: ShopSuccessPage,
});

const userRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/$token",
  component: UserPage,
});

const statRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/stat",
  component: StatPage,
});

/* ── Route tree & router ──────────────────────────────────────── */

const routeTree = rootRoute.addChildren([
  indexRoute,
  panelIndexRoute,
  setupRoute,
  loginRoute,
  dashboardRoute,
  usersRoute,
  nodesRoute,
  inboundsRoute,
  outboundsRoute,
  routerulesRoute,
  settingsRoute,
  sniproxyRoute,
  plansRoute,
  announcementsRoute,
  ticketsRoute,
  domainsRoute,
  userGroupsRoute,
  tracerouteRoute,
  latencyRoute,
  auditRoute,
  shopRoute,
  shopSuccessRoute,
  shopTestRoute,
  shopTestSuccessRoute,
  userRoute,
  statRoute,
]);

export const router = createRouter({ routeTree });

/* ── Type registration ────────────────────────────────────────── */

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
