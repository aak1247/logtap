import { useSyncExternalStore, type ReactNode } from "react";
import { Navigate, createBrowserRouter } from "react-router-dom";
import { RootLayout } from "./ui/RootLayout";
import { DashboardPage } from "./ui/pages/DashboardPage";
import { AnalyticsPage } from "./ui/pages/AnalyticsPage";
import { EventsPage } from "./ui/pages/EventsPage";
import { EventDetailPage } from "./ui/pages/EventDetailPage";
import { LogsPage } from "./ui/pages/LogsPage";
import { LoginPage } from "./ui/pages/LoginPage";
import { ProjectsPage } from "./ui/pages/ProjectsPage";
import { BootstrapPage } from "./ui/pages/BootstrapPage";
import { DocsPage } from "./ui/pages/DocsPage";
import { loadSettings, subscribeSettingsChange } from "./lib/storage";

function RequireAuth(props: { children: ReactNode }) {
  const s = useSyncExternalStore(subscribeSettingsChange, loadSettings, loadSettings);
  if (!s.token) return <Navigate to="/login" replace />;
  return <>{props.children}</>;
}

export const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  { path: "/bootstrap", element: <BootstrapPage /> },
  { path: "/docs/*", element: <DocsPage /> },
  {
    path: "/",
    element: (
      <RequireAuth>
        <RootLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <DashboardPage /> },
      { path: "projects", element: <ProjectsPage /> },
      { path: "analytics", element: <AnalyticsPage /> },
      { path: "events", element: <EventsPage /> },
      { path: "events/:eventId", element: <EventDetailPage /> },
      { path: "logs", element: <LogsPage /> },
    ],
  },
]);
