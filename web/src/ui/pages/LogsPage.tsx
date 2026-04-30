import { useEffect, useMemo, useRef, useState } from "react";
import { loadSettings } from "../../lib/storage";
import { searchLogs, searchUnified, type LogRow } from "../../lib/api";
import { Panel } from "../components/Panel";
import { TimeRangePicker } from "../components/DateTimePicker";
import { useNavigate, useSearchParams } from "react-router-dom";

const STORAGE_KEY_SAVED_QUERIES = "logtap_saved_queries";

type SavedQuery = { name: string; q: string };

function loadSavedQueries(): SavedQuery[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY_SAVED_QUERIES) || "[]");
  } catch {
    return [];
  }
}

function saveSavedQueries(qs: SavedQuery[]) {
  localStorage.setItem(STORAGE_KEY_SAVED_QUERIES, JSON.stringify(qs));
}

export function LogsPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [searchParams] = useSearchParams();
  const autoRan = useRef(false);
  const [q, setQ] = useState(() => searchParams.get("q") || "");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [rows, setRows] = useState<LogRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  const [showHelp, setShowHelp] = useState(false);
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>(loadSavedQueries);
  const [facets, setFacets] = useState<Record<string, { key: string; count: number }[]> | null>(null);

  useEffect(() => {
    if (settings.token) {
      if (!settings.projectId) nav("/projects");
    } else if (!settings.projectId) {
      nav("/login");
    }
  }, [settings.token, settings.projectId, nav]);

  useEffect(() => {
    if (autoRan.current) return;
    if (!settings.token || !settings.projectId) return;
    autoRan.current = true;
    const urlQ = searchParams.get("q");
    if (urlQ) setQ(urlQ);
    void run();
  }, [settings.token, settings.projectId]);

  async function run() {
    try {
      setLoading(true);
      setErr("");
      setFacets(null);
      if (!settings.token || !settings.projectId) return;

      // Try unified search first (returns facets), fall back to legacy
      try {
        const result = await searchUnified(settings, {
          q: q.trim() || undefined,
          start: start.trim() || undefined,
          end: end.trim() || undefined,
          page: 1,
          pageSize: 200,
        });
        setRows(result.items || []);
        setFacets(result.facets || null);
      } catch {
        // fallback to legacy API
        const data = await searchLogs(settings, {
          q: q.trim() || undefined,
          start: start.trim() || undefined,
          end: end.trim() || undefined,
          limit: 200,
        });
        setRows(data);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  function handleSaveQuery() {
    const name = q.trim().slice(0, 40);
    if (!name) return;
    const next = [...savedQueries, { name, q: q.trim() }];
    saveSavedQueries(next);
    setSavedQueries(next);
  }

  function handleDeleteSaved(idx: number) {
    const next = savedQueries.filter((_, i) => i !== idx);
    saveSavedQueries(next);
    setSavedQueries(next);
  }

  // Extract keywords from query for highlighting
  const highlightTerms = useMemo(() => {
    const terms: string[] = [];
    // Extract values from DSL: key:value patterns
    const matches = q.matchAll(/(?:message|msg|q):(\S+)/gi);
    for (const m of matches) terms.push(m[1]);
    // Free text (non-keyword parts)
    const freeText = q.replace(/\w+:\S+/g, "").trim();
    if (freeText) terms.push(freeText);
    return terms;
  }, [q]);

  return (
    <div className="flex gap-4">
      {/* Facets sidebar */}
      {facets && Object.keys(facets).length > 0 && (
        <div className="hidden w-48 shrink-0 lg:block">
          <Panel title="Facets">
            {Object.entries(facets).map(([field, items]) => (
              <div key={field} className="mb-3">
                <div className="mb-1 text-xs font-medium text-zinc-400">{field}</div>
                {items.map((it) => (
                  <button
                    key={it.key}
                    className="flex w-full items-center justify-between rounded px-2 py-1 text-xs text-zinc-300 hover:bg-zinc-900"
                    onClick={() => setQ((prev) => `${prev} ${field}:${it.key}`.trim())}
                  >
                    <span>{it.key || "(empty)"}</span>
                    <span className="text-zinc-500">{it.count}</span>
                  </button>
                ))}
              </div>
            ))}
          </Panel>
        </div>
      )}

      <div className="flex-1 space-y-4">
        <div>
          <div className="text-lg font-semibold">日志搜索</div>
          <div className="mt-1 text-sm text-zinc-400">
            支持 DSL 语法：level:error tag:api message:timeout
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
            <div className="flex gap-2">
              <button
                className="text-xs text-zinc-400 hover:text-zinc-200"
                onClick={() => setShowHelp((v) => !v)}
                title="搜索语法帮助"
              >
                ?
              </button>
              <button
                className="btn btn-md btn-primary"
                onClick={run}
                disabled={loading}
              >
                {loading ? "查询中..." : "查询"}
              </button>
            </div>
          }
        >
          <div className="space-y-3">
            <div>
              <div className="text-xs text-zinc-400">搜索</div>
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="level:error tag:api message:timeout"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
                onKeyDown={(e) => e.key === "Enter" && run()}
              />
            </div>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              <div className="md:col-span-2">
                <TimeRangePicker
                  label="时间范围"
                  start={start}
                  end={end}
                  onStartChange={setStart}
                  onEndChange={setEnd}
                />
              </div>
            </div>
            <div className="flex items-center gap-2">
              <button
                className="text-xs text-indigo-400 hover:text-indigo-300"
                onClick={handleSaveQuery}
                disabled={!q.trim()}
              >
                保存查询
              </button>
            </div>
          </div>
        </Panel>

        {showHelp && (
          <Panel title="搜索语法帮助">
            <div className="space-y-2 text-xs text-zinc-300">
              <div>
                <code className="text-indigo-300">level:error</code> — 按日志级别过滤
              </div>
              <div>
                <code className="text-indigo-300">tag:api</code> — 按 tag 过滤
              </div>
              <div>
                <code className="text-indigo-300">message:timeout</code> — 在消息中搜索关键词
              </div>
              <div>
                <code className="text-indigo-300">trace_id:abc123</code> — 按 trace ID 搜索
              </div>
              <div>多个条件用空格分隔，表示 AND 关系。</div>
              <div>不带 key 的文本会在 message 中全文搜索。</div>
            </div>
          </Panel>
        )}

        {savedQueries.length > 0 && (
          <Panel title="保存的查询">
            <div className="flex flex-wrap gap-2">
              {savedQueries.map((sq, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1 rounded-full bg-zinc-900 px-3 py-1 text-xs text-zinc-300"
                >
                  <button
                    className="hover:text-indigo-300"
                    onClick={() => {
                      setQ(sq.q);
                      void run();
                    }}
                  >
                    {sq.name}
                  </button>
                  <button
                    className="text-zinc-500 hover:text-red-400"
                    onClick={() => handleDeleteSaved(i)}
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          </Panel>
        )}

        <Panel title={`结果（${rows.length}）`}>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
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
                    <td className="py-1.5 pr-4 font-mono text-[11px] text-zinc-400">
                      {new Date(r.timestamp).toLocaleString()}
                    </td>
                    <td className="py-1.5 pr-4">
                      <LevelPill level={r.level} />
                    </td>
                    <td className="py-1.5 pr-4 font-mono text-[11px] text-zinc-500">
                      <div>{r.trace_id ?? ""}</div>
                      <div>{r.span_id ?? ""}</div>
                    </td>
                    <td className="py-1.5 pr-4 text-zinc-100">
                      <div
                        className="text-xs leading-5"
                        dangerouslySetInnerHTML={{
                          __html: highlightText(r.message, highlightTerms),
                        }}
                      />
                      {r.fields ? (
                        <details className="group mt-1.5">
                          <summary className="cursor-pointer select-none text-[11px] text-zinc-400 marker:text-zinc-600 hover:text-zinc-200">
                            fields <span className="text-zinc-500">({countKeys(r.fields)})</span>{" "}
                            <span className="group-open:hidden">展开</span>
                            <span className="hidden group-open:inline">收起</span>
                            <span className="ml-2 text-zinc-500">{fieldsPreview(r.fields)}</span>
                          </summary>
                          <pre className="mt-2 max-h-64 overflow-auto rounded-md bg-zinc-950/40 p-2 font-mono text-[11px] leading-4 text-zinc-200 ring-1 ring-zinc-900">
                            {JSON.stringify(r.fields, null, 2)}
                          </pre>
                        </details>
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
    </div>
  );
}

function highlightText(text: string, terms: string[]): string {
  if (!terms.length || !text) return escapeHtml(text);
  let result = escapeHtml(text);
  for (const term of terms) {
    if (!term) continue;
    const escaped = escapeHtml(term);
    const regex = new RegExp(`(${escapeRegex(escaped)})`, "gi");
    result = result.replace(regex, '<mark class="bg-yellow-700/50 text-yellow-200 rounded px-0.5">$1</mark>');
  }
  return result;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function countKeys(obj: Record<string, unknown>): number {
  try {
    return Object.keys(obj).length;
  } catch {
    return 0;
  }
}

function fieldsPreview(obj: Record<string, unknown>): string {
  try {
    const s = JSON.stringify(obj);
    if (!s) return "";
    const max = 120;
    return s.length > max ? `${s.slice(0, max)}…` : s;
  } catch {
    return "";
  }
}

function LevelPill(props: { level?: string }) {
  const level = (props.level || "").toLowerCase();
  const palette =
    level === "error" || level === "fatal"
      ? "bg-red-950/50 text-red-200 ring-red-900/60"
      : level === "warn" || level === "warning"
        ? "bg-amber-950/40 text-amber-200 ring-amber-900/60"
        : level === "debug"
          ? "bg-zinc-950/40 text-zinc-300 ring-zinc-800"
          : level
            ? "bg-indigo-950/40 text-indigo-200 ring-indigo-900/60"
            : "bg-zinc-950/40 text-zinc-400 ring-zinc-800";

  return (
    <span className={`inline-flex items-center rounded-md px-2 py-0.5 font-mono text-[11px] ring-1 ${palette}`}>
      {props.level || ""}
    </span>
  );
}
