import { useSyncExternalStore, type ReactNode } from "react";
import { Link, NavLink, Outlet } from "react-router-dom";
import { loadSettings, subscribeSettingsChange } from "../lib/storage";

const navItem =
  "px-3 py-2 rounded-md text-sm text-zinc-300 hover:text-zinc-100 hover:bg-zinc-900";
const navItemActive = "bg-zinc-900 text-zinc-100";

export function RootLayout(props?: {
  extraNavItems?: ReactNode;
  allowApiBaseEdit?: boolean;
}) {
  const s = useSyncExternalStore(subscribeSettingsChange, loadSettings, loadSettings);
  const extraNavItems: ReactNode = props?.extraNavItems ?? null;
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-20 border-b border-zinc-900 bg-zinc-950/90 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 rounded-lg bg-gradient-to-br from-indigo-500 to-cyan-500" />
            <div className="flex flex-col leading-tight">
              <div className="text-sm font-semibold">logtap 控制台</div>
              <div className="flex items-center gap-2 text-xs text-zinc-400">
                <span>{s.projectId ? `项目 ${s.projectId}` : "未选择项目"}</span>
                <Link
                  to="/projects"
                  className="rounded-md border border-zinc-800 bg-zinc-950 px-2 py-0.5 text-[11px] text-zinc-300 hover:bg-zinc-900 hover:text-zinc-100"
                >
                  切换
                </Link>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <nav className="flex items-center gap-1">
              <NavLink
                to="/"
                end
                className={({ isActive }) =>
                  `${navItem} ${isActive ? navItemActive : ""}`
                }
              >
                概览
              </NavLink>
              <NavLink
                to="/analytics"
                className={({ isActive }) =>
                  `${navItem} ${isActive ? navItemActive : ""}`
                }
              >
                分析
              </NavLink>
              <NavLink
                to="/events"
                className={({ isActive }) =>
                  `${navItem} ${isActive ? navItemActive : ""}`
                }
              >
                事件
              </NavLink>
              <NavLink
                to="/logs"
                className={({ isActive }) =>
                  `${navItem} ${isActive ? navItemActive : ""}`
                }
              >
                日志
              </NavLink>
              <Link to="/docs" className={navItem}>
                文档
              </Link>
              <NavLink
                to="/settings"
                className={({ isActive }) =>
                  `${navItem} ${isActive ? navItemActive : ""}`
                }
              >
                设置
              </NavLink>
              {extraNavItems}
            </nav>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
