import { useEffect, useSyncExternalStore } from "react";
import { useNavigate } from "react-router-dom";
import { loadSettings, subscribeSettingsChange } from "../../lib/storage";
import { MonitorTab } from "./alerts/MonitorTab";

export function MonitorsPage() {
  const settings = useSyncExternalStore(subscribeSettingsChange, loadSettings, loadSettings);
  const nav = useNavigate();

  useEffect(() => {
    if (!settings.token) {
      nav("/login");
      return;
    }
    if (!settings.projectId) {
      nav("/projects");
    }
  }, [settings.token, settings.projectId, nav]);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="text-lg font-semibold">监控</div>
          <div className="mt-1 text-sm text-zinc-500">
            管理项目级检查任务、运行历史和可用的 Detector 类型。
          </div>
        </div>
      </div>

      <MonitorTab settings={settings} />
    </div>
  );
}
