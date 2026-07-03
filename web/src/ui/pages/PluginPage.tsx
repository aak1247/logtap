import { useEffect, useMemo, useState } from "react";
import { useLocation } from "react-router-dom";
import { listPluginViews } from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { extensionFromApi } from "../pluginExtensions/api";
import type { PluginExtensionDescriptor } from "../pluginExtensions/registry";
import { pluginExtensionRegistry } from "../pluginExtensions/registry";
import { PluginAnalysisView } from "../pluginExtensions/PluginAnalysisView";

export function PluginPage() {
  const location = useLocation();
  const settings = useMemo(() => loadSettings(), []);
  const [remotePages, setRemotePages] = useState<PluginExtensionDescriptor[]>([]);
  const [remotePagesLoaded, setRemotePagesLoaded] = useState(false);
  const [pluginViewsVersion, setPluginViewsVersion] = useState(0);
  const page =
    (remotePagesLoaded
      ? remotePages
      : mergeExtensions(pluginExtensionRegistry.getPages(), remotePages)
    ).find(
      (item) => item.path === location.pathname,
    ) ?? pluginExtensionRegistry.getByPath(location.pathname);

  useEffect(() => {
    const onChanged = () => setPluginViewsVersion((v) => v + 1);
    window.addEventListener("plugin-settings-changed", onChanged);
    return () => window.removeEventListener("plugin-settings-changed", onChanged);
  }, []);

  useEffect(() => {
    if (!settings.token || !settings.projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await listPluginViews(settings);
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
  }, [settings.apiBase, settings.token, settings.projectId, pluginViewsVersion]);

  if (!page) {
    return (
      <div className="rounded-xl border border-zinc-800 bg-zinc-950 p-4 text-sm text-zinc-500">
        插件页面不存在
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <div className="text-lg font-semibold">{page.title}</div>
        <div className="mt-1 text-sm text-zinc-400">{page.packageId}</div>
      </div>
      <PluginAnalysisView extension={page} settings={settings} />
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
