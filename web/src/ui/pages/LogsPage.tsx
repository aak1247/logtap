import { useEffect, useMemo, useState } from "react";
import { loadSettings } from "../../lib/storage";
import { searchLogs, type LogRow } from "../../lib/api";
import { Panel } from "../components/Panel";
import { useNavigate } from "react-router-dom";

export function LogsPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [q, setQ] = useState("");
  const [traceId, setTraceId] = useState("");
  const [level, setLevel] = useState("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [limit, setLimit] = useState(200);
  const [rows, setRows] = useState<LogRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (settings.token) {
      if (!settings.projectId) nav("/projects");
    } else if (!settings.projectId) {
      nav("/login");
    }
  }, [settings.token, settings.projectId, nav]);

  async function run() {
    try {
      setLoading(true);
      setErr("");
      if (!settings.token || !settings.projectId) return;
      const data = await searchLogs(settings, {
        q: q.trim() || undefined,
        trace_id: traceId.trim() || undefined,
        level: level.trim() || undefined,
        start: start.trim() || undefined,
        end: end.trim() || undefined,
        limit,
      });
      setRows(data);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <div className="text-lg font-semibold">日志搜索</div>
        <div className="mt-1 text-sm text-zinc-400">
          支持全文：q + trace_id + 时间范围（RFC3339）
        </div>
      </div>

      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel
        title="查询"
        right={
          <button
            className="rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
            onClick={run}
            disabled={loading}
          >
            {loading ? "查询中..." : "查询"}
          </button>
        }
      >
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <Field label="q" value={q} onChange={setQ} placeholder="payment failed" />
          <Field label="trace_id" value={traceId} onChange={setTraceId} placeholder="abc123" />
          <Field label="level" value={level} onChange={setLevel} placeholder="info/error" />
          <Field label="start" value={start} onChange={setStart} placeholder="2025-01-01T00:00:00Z" />
          <Field label="end" value={end} onChange={setEnd} placeholder="2025-01-01T23:59:59Z" />
          <Field
            label="limit"
            value={String(limit)}
            onChange={(v) => setLimit(Number(v || "200"))}
            placeholder="200"
          />
        </div>
      </Panel>

      <Panel title={`结果（${rows.length}）`}>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-zinc-500">
              <tr>
                <th className="py-2 pr-4">时间</th>
                <th className="py-2 pr-4">级别</th>
                <th className="py-2 pr-4">trace/span</th>
                <th className="py-2 pr-4">message</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {rows.map((r) => (
                <tr key={r.id} className="align-top hover:bg-zinc-900/40">
                  <td className="py-2 pr-4 text-zinc-300">
                    {new Date(r.timestamp).toLocaleString()}
                  </td>
                  <td className="py-2 pr-4 text-zinc-300">{r.level ?? ""}</td>
                  <td className="py-2 pr-4 font-mono text-xs text-zinc-500">
                    <div>{r.trace_id ?? ""}</div>
                    <div>{r.span_id ?? ""}</div>
                  </td>
                  <td className="py-2 pr-4 text-zinc-100">
                    <div>{r.message}</div>
                    {r.fields ? (
                      <pre className="mt-2 overflow-auto rounded bg-zinc-900/40 p-2 text-xs text-zinc-300">
                        {JSON.stringify(r.fields, null, 2)}
                      </pre>
                    ) : null}
                  </td>
                </tr>
              ))}
              {rows.length === 0 ? (
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

function Field(props: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  return (
    <div>
      <div className="text-xs text-zinc-400">{props.label}</div>
      <input
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        placeholder={props.placeholder}
        className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
      />
    </div>
  );
}
