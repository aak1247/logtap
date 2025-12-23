import { useEffect, useMemo, useState } from "react";
import { getRecentEvents, type RecentEvent } from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { Panel } from "../components/Panel";
import { Link, useNavigate } from "react-router-dom";

export function EventsPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [limit, setLimit] = useState(100);
  const [events, setEvents] = useState<RecentEvent[]>([]);
  const [err, setErr] = useState("");

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
        const data = await getRecentEvents(settings, limit);
        if (!cancelled) setEvents(data);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId, limit]);

  return (
    <div className="space-y-4">
      <div>
        <div className="text-lg font-semibold">事件</div>
        <div className="mt-1 text-sm text-zinc-400">
          最近 {limit} 条（按时间倒序）
        </div>
      </div>

      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel
        title="列表"
        right={
          <div className="flex items-center gap-2">
            <label className="text-xs text-zinc-400">Limit</label>
            <input
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value || "100"))}
              type="number"
              min={1}
              max={500}
              className="w-24 rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-sm outline-none focus:border-indigo-500"
            />
          </div>
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
