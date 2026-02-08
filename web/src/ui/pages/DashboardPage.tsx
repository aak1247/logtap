import { useEffect, useMemo, useState } from "react";
import {
  getMetricsToday,
  getMetricsTotal,
  getRecentEvents,
  getStorageEstimate,
  type MetricsToday,
  type MetricsTotal,
  type RecentEvent,
  type StorageEstimate,
} from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { Panel } from "../components/Panel";
import { StatCard } from "../components/StatCard";
import { Link, useNavigate } from "react-router-dom";

export function DashboardPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [metrics, setMetrics] = useState<MetricsToday | null>(null);
  const [total, setTotal] = useState<MetricsTotal | null>(null);
  const [storage, setStorage] = useState<StorageEstimate | null>(null);
  const [events, setEvents] = useState<RecentEvent[]>([]);
  const [err, setErr] = useState<string>("");

  useEffect(() => {
    if (settings.token) {
      if (!settings.projectId) nav("/projects");
    } else if (!settings.projectId) {
      nav("/login");
    }
  }, [settings.token, settings.projectId, nav]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setErr("");
        const [m, t, se, e] = await Promise.all([
          getMetricsToday(settings).catch(() => null),
          getMetricsTotal(settings).catch(() => null),
          getStorageEstimate(settings).catch(() => null),
          getRecentEvents(settings, 20),
        ]);
        if (cancelled) return;
        setMetrics(m);
        setTotal(t);
        setStorage(se);
        setEvents(e);
      } catch (e) {
        if (cancelled) return;
        setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId]);

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between">
        <div>
          <div className="text-lg font-semibold">概览</div>
          <div className="mt-1 text-sm text-zinc-400">
            API：{settings.apiBase} / 项目：{settings.projectId}
          </div>
        </div>
        <div className="text-xs text-zinc-500">刷新页面即可更新</div>
      </div>

      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <StatCard
          title="今日日志"
          value={metrics ? metrics.logs : "—"}
          hint={metrics ? metrics.date : "未启用 metrics 或 Redis 不可用"}
        />
        <StatCard
          title="今日事件"
          value={metrics ? metrics.events : "—"}
        />
        <StatCard title="今日错误" value={metrics ? metrics.errors : "—"} />
        <StatCard title="今日用户(去重)" value={metrics ? metrics.users : "—"} />
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <StatCard title="累计日志" value={total ? total.logs : "—"} />
        <StatCard title="累计事件" value={total ? total.events : "—"} />
        <StatCard title="累计用户(约,去重)" value={total ? total.users : "—"} />
      </div>

      <Panel
        title="存储"
        right={
          <Link className="text-sm text-indigo-400 hover:text-indigo-300" to="/settings#cleanup">
            清理设置 →
          </Link>
        }
      >
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          <StatCard
            title="日志存储(估计)"
            value={storage ? formatBytes(storage.logs.est_bytes) : "—"}
            hint={storage ? `${storage.logs.count} 条 / avg ${storage.logs.avg_row_bytes} B` : ""}
          />
          <StatCard
            title="事件存储(估计)"
            value={storage ? formatBytes(storage.events.est_bytes) : "—"}
            hint={storage ? `${storage.events.count} 条 / avg ${storage.events.avg_row_bytes} B` : ""}
          />
          <StatCard
            title="合计(估计)"
            value={storage ? formatBytes(storage.total_bytes) : "—"}
            hint={storage ? new Date(storage.estimated_at).toLocaleString() : ""}
          />
        </div>
      </Panel>

      <Panel
        title="最新事件"
        right={
          <Link className="text-sm text-indigo-400 hover:text-indigo-300" to="/events">
            查看全部 →
          </Link>
        }
      >
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-zinc-500">
              <tr>
                <th className="py-2 pr-4">时间</th>
                <th className="py-2 pr-4">级别</th>
                <th className="py-2 pr-4">标题</th>
                <th className="py-2 pr-4">ID</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {events.map((e) => (
                <tr key={e.id} className="hover:bg-zinc-900/40">
                  <td className="py-2 pr-4 text-zinc-300">
                    {new Date(e.timestamp).toLocaleString()}
                  </td>
                  <td className="py-2 pr-4 text-zinc-300">{e.level ?? ""}</td>
                  <td className="py-2 pr-4 text-zinc-100">
                    <Link className="hover:underline" to={`/events/${e.id}`}>
                      {e.title ?? "(no title)"}
                    </Link>
                  </td>
                  <td className="py-2 pr-4 font-mono text-xs text-zinc-500">
                    {e.id}
                  </td>
                </tr>
              ))}
              {events.length === 0 ? (
                <tr>
                  <td className="py-6 text-sm text-zinc-500" colSpan={4}>
                    暂无数据
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  const digits = i === 0 ? 0 : v >= 10 ? 1 : 2;
  return `${v.toFixed(digits)} ${units[i]}`;
}
