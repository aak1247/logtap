import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { getEvent } from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { Panel } from "../components/Panel";

export function EventDetailPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const { eventId } = useParams();
  const [data, setData] = useState<unknown>(null);
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
        setErr("");
        if (!eventId) return;
        if (!settings.token || !settings.projectId) return;
        const d = await getEvent(settings, eventId);
        if (!cancelled) setData(d);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId, eventId]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-lg font-semibold">事件详情</div>
          <div className="mt-1 font-mono text-xs text-zinc-500">{eventId}</div>
        </div>
        <Link className="text-sm text-indigo-400 hover:text-indigo-300" to="/events">
          ← 返回列表
        </Link>
      </div>

      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel title="JSON">
        <pre className="max-h-[70vh] overflow-auto rounded-lg bg-zinc-900/40 p-4 text-xs leading-relaxed text-zinc-100">
          {data ? JSON.stringify(data, null, 2) : "加载中..."}
        </pre>
      </Panel>
    </div>
  );
}
