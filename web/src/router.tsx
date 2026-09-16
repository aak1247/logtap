import { lazy, Suspense, useSyncExternalStore, type ReactNode } from "react";
import { Navigate, createBrowserRouter } from "react-router-dom";
import { RootLayout } from "./ui/RootLayout";
import { loadSettings, subscribeSettingsChange } from "./lib/storage";
import "./ui/pluginExtensions/builtins";
import "./ui/widgets/builtins";

// Route-level code splitting: each page is its own chunk so the initial
// bundle only carries the layout. The docs page additionally pulls in
// react-markdown and all markdown sources, so it must not be in the entry.
const LoginPage = lazy(() =>
  import("./ui/pages/LoginPage").then((m) => ({ default: m.LoginPage })),
);
const BootstrapPage = lazy(() =>
  import("./ui/pages/BootstrapPage").then((m) => ({ default: m.BootstrapPage })),
);
const DocsPage = lazy(() =>
  import("./ui/pages/DocsPage").then((m) => ({ default: m.DocsPage })),
);
const DashboardPage = lazy(() =>
  import("./ui/pages/DashboardPage").then((m) => ({ default: m.DashboardPage })),
);
const ProjectsPage = lazy(() =>
  import("./ui/pages/ProjectsPage").then((m) => ({ default: m.ProjectsPage })),
);
const AnalyticsPage = lazy(() =>
  import("./ui/pages/AnalyticsPage").then((m) => ({ default: m.AnalyticsPage })),
);
const DistributionPage = lazy(() =>
  import("./ui/pages/DistributionPage").then((m) => ({
    default: m.DistributionPage,
  })),
);
const EventsPage = lazy(() =>
  import("./ui/pages/EventsPage").then((m) => ({ default: m.EventsPage })),
);
const EventDetailPage = lazy(() =>
  import("./ui/pages/EventDetailPage").then((m) => ({
    default: m.EventDetailPage,
  })),
);
const LogsPage = lazy(() =>
  import("./ui/pages/LogsPage").then((m) => ({ default: m.LogsPage })),
);
const MonitorsPage = lazy(() =>
  import("./ui/pages/MonitorsPage").then((m) => ({ default: m.MonitorsPage })),
);
const AlertsPage = lazy(() =>
  import("./ui/pages/AlertsPage").then((m) => ({ default: m.AlertsPage })),
);
const PluginPage = lazy(() =>
  import("./ui/pages/PluginPage").then((m) => ({ default: m.PluginPage })),
);
const SettingsPage = lazy(() =>
  import("./ui/pages/SettingsPage").then((m) => ({ default: m.SettingsPage })),
);

function PageFallback() {
  return (
    <div className="flex h-32 items-center justify-center text-sm text-zinc-500">
      加载中…
    </div>
  );
}

function page(node: ReactNode) {
  return <Suspense fallback={<PageFallback />}>{node}</Suspense>;
}

function RequireAuth(props: { children: ReactNode }) {
  const s = useSyncExternalStore(
    subscribeSettingsChange,
    loadSettings,
    loadSettings,
  );
  if (!s.token) return <Navigate to="/login" replace />;
  return <>{props.children}</>;
}

export const router = createBrowserRouter([
  { path: "/login", element: page(<LoginPage />) },
  { path: "/bootstrap", element: page(<BootstrapPage />) },
  { path: "/docs/*", element: page(<DocsPage />) },
  {
    path: "/",
    element: (
      <RequireAuth>
        <RootLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: page(<DashboardPage />) },
      { path: "projects", element: page(<ProjectsPage />) },
      { path: "analytics", element: page(<AnalyticsPage />) },
      { path: "analytics/distribution", element: page(<DistributionPage />) },
      { path: "events", element: page(<EventsPage />) },
      { path: "events/:eventId", element: page(<EventDetailPage />) },
      { path: "logs", element: page(<LogsPage />) },
      { path: "monitors", element: page(<MonitorsPage />) },
      { path: "alerts", element: page(<AlertsPage />) },
      { path: "plugins/:pluginId", element: page(<PluginPage />) },
      { path: "settings", element: page(<SettingsPage />) },
      { path: "settings/:section", element: page(<SettingsPage />) },
    ],
  },
]);
