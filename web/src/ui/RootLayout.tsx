import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { Link, NavLink, Outlet } from "react-router-dom";
import { listPluginViews } from "../lib/api";
import { loadSettings, subscribeSettingsChange } from "../lib/storage";
import { extensionFromApi } from "./pluginExtensions/api";
import type { PluginExtensionDescriptor } from "./pluginExtensions/registry";
import { pluginExtensionRegistry } from "./pluginExtensions/registry";

const navItem =
  "px-3 py-2 rounded-lg text-sm text-zinc-300 transition-colors hover:text-zinc-100 hover:bg-zinc-900";
const navItemActive = "bg-zinc-900 text-zinc-100";

export function RootLayout(props?: {
  extraNavItems?: ReactNode;
  allowApiBaseEdit?: boolean;
  collapseSettingsToUserMenu?: boolean;
  userMenuItems?: ReactNode;
}) {
  const s = useSyncExternalStore(subscribeSettingsChange, loadSettings, loadSettings);
  const extraNavItems: ReactNode = props?.extraNavItems ?? null;
  const userMenuItems: ReactNode = props?.userMenuItems ?? null;
  const [remotePages, setRemotePages] = useState<PluginExtensionDescriptor[]>([]);
  const [remotePagesLoaded, setRemotePagesLoaded] = useState(false);
  const [pluginViewsVersion, setPluginViewsVersion] = useState(0);
  const pluginPages = remotePagesLoaded
    ? remotePages
    : mergeExtensions(pluginExtensionRegistry.getPages(), remotePages);

  useEffect(() => {
    const onChanged = () => setPluginViewsVersion((v) => v + 1);
    window.addEventListener("plugin-settings-changed", onChanged);
    return () => window.removeEventListener("plugin-settings-changed", onChanged);
  }, []);

  useEffect(() => {
    if (!s.token) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await listPluginViews(s);
        if (cancelled) return;
        setRemotePages(
          res.items
            .map(extensionFromApi)
            .filter((item): item is PluginExtensionDescriptor => Boolean(item))
            .filter((item) => item.surface === "page"),
        );
        setRemotePagesLoaded(true);
      } catch {
        if (!cancelled) {
          setRemotePages([]);
          setRemotePagesLoaded(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [s.apiBase, s.token, s.projectId, pluginViewsVersion]);
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
                  className="btn btn-xs btn-outline text-[11px]"
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
              <NavLink
                to="/alerts"
                className={({ isActive }) =>
                  `${navItem} ${isActive ? navItemActive : ""}`
                }
              >
                报警
              </NavLink>
              <Link to="/docs" className={navItem}>
                文档
              </Link>
              {!props?.collapseSettingsToUserMenu ? (
                <NavLink
                  to="/settings"
                  className={({ isActive }) =>
                    `${navItem} ${isActive ? navItemActive : ""}`
                  }
                >
                  设置
                </NavLink>
              ) : null}
              {pluginPages.map((page) =>
                page.path ? (
                  <NavLink
                    key={page.id}
                    to={page.path}
                    className={({ isActive }) =>
                      `${navItem} ${isActive ? navItemActive : ""}`
                    }
                  >
                    {page.title}
                  </NavLink>
                ) : null,
              )}
              {extraNavItems}
            </nav>
            {props?.collapseSettingsToUserMenu || userMenuItems ? (
              <details className="relative">
                <summary
                  className="flex h-9 w-9 cursor-pointer list-none items-center justify-center rounded-full bg-zinc-900 text-xs font-semibold text-zinc-100 ring-1 ring-zinc-800 transition-colors hover:bg-zinc-800 [&::-webkit-details-marker]:hidden"
                  title="账号菜单"
                >
                  LT
                </summary>
                <div className="absolute right-0 mt-2 w-44 overflow-hidden rounded-md border border-zinc-800 bg-zinc-950 py-1 shadow-xl shadow-black/30">
                  {props?.collapseSettingsToUserMenu ? (
                    <NavLink
                      to="/settings"
                      className={({ isActive }) =>
                        `block px-3 py-2 text-sm transition-colors ${
                          isActive
                            ? "bg-zinc-900 text-zinc-100"
                            : "text-zinc-300 hover:bg-zinc-900 hover:text-zinc-100"
                        }`
                      }
                    >
                      设置
                    </NavLink>
                  ) : null}
                  {userMenuItems}
                </div>
              </details>
            ) : null}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}

function mergeExtensions(
  local: PluginExtensionDescriptor[],
  remote: PluginExtensionDescriptor[],
): PluginExtensionDescriptor[] {
  const map = new Map<string, PluginExtensionDescriptor>();
  for (const item of local) map.set(item.id, item);
  for (const item of remote) map.set(item.id, item);
  return Array.from(map.values()).sort((a, b) => a.title.localeCompare(b.title));
}
