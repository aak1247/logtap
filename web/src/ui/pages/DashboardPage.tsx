import { useEffect, useMemo, useState } from "react";
import {
  cleanupEventsBefore,
  cleanupLogsBefore,
  getMetricsToday,
  getMetricsTotal,
  getCleanupPolicy,
  getRecentEvents,
  getStorageEstimate,
  runCleanupPolicy,
  upsertCleanupPolicy,
  type CleanupPolicy,
  type MetricsToday,
  type MetricsTotal,
  type RecentEvent,
  type StorageEstimate,
} from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { clampFunnelDays, loadFunnelDays } from "../../lib/prefs";
import { Panel } from "../components/Panel";
import { StatCard } from "../components/StatCard";
import { Link, useNavigate } from "react-router-dom";

export function DashboardPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const funnelDays = useMemo(() => clampFunnelDays(loadFunnelDays()), []);
  const [metrics, setMetrics] = useState<MetricsToday | null>(null);
  const [total, setTotal] = useState<MetricsTotal | null>(null);
  const [storage, setStorage] = useState<StorageEstimate | null>(null);
  const [policy, setPolicy] = useState<CleanupPolicy | null>(null);
  const [policyDraft, setPolicyDraft] = useState<{
    enabled: boolean;
    logsDays: string;
    eventsDays: string;
    trackEventsDays: string;
    hourUTC: string;
    minuteUTC: string;
  } | null>(null);
  const [cleanupBusy, setCleanupBusy] = useState(false);
  const [cleanupMsg, setCleanupMsg] = useState("");
  const [manualBeforeLocal, setManualBeforeLocal] = useState(() =>
    toDateTimeLocal(new Date(Date.now() - 30 * 24 * 3600 * 1000)),
  );
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
        const [m, t, se, cp, e] = await Promise.all([
          getMetricsToday(settings).catch(() => null),
          getMetricsTotal(settings).catch(() => null),
          getStorageEstimate(settings).catch(() => null),
          getCleanupPolicy(settings).catch(() => null),
          getRecentEvents(settings, 20),
        ]);
        if (cancelled) return;
        setMetrics(m);
        setTotal(t);
        setStorage(se);
        setPolicy(cp);
        if (!policyDraft) {
          setPolicyDraft({
            enabled: cp?.enabled ?? false,
            logsDays: String(cp?.logs_retention_days ?? 30),
            eventsDays: String(cp?.events_retention_days ?? 30),
            trackEventsDays: String(cp?.track_events_retention_days ?? 0),
            hourUTC: String(cp?.schedule_hour_utc ?? 3),
            minuteUTC: String(cp?.schedule_minute_utc ?? 0),
          });
        }
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

      <Panel title="存储与清理">
        {cleanupMsg ? (
          <div className="mb-3 rounded-lg border border-zinc-900 bg-zinc-950/60 p-3 text-sm text-zinc-200">
            {cleanupMsg}
          </div>
        ) : null}

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

        <div className="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2">
          <div className="rounded-xl border border-zinc-900 bg-zinc-950 p-4">
            <div className="text-sm font-semibold">自动清理</div>
            <div className="mt-1 text-xs text-zinc-500">
              通过保留天数定期删除更早的数据（UTC 定时）。
            </div>

            {!policyDraft ? (
              <div className="mt-3 text-sm text-zinc-500">未加载清理策略</div>
            ) : (
              <div className="mt-4 space-y-3">
                <label className="flex items-center justify-between gap-3 text-sm">
                  <span className="text-zinc-300">启用</span>
                  <input
                    type="checkbox"
                    checked={policyDraft.enabled}
                    onChange={(e) =>
                      setPolicyDraft((prev) =>
                        prev ? { ...prev, enabled: e.target.checked } : prev,
                      )
                    }
                  />
                </label>

                <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <Field
                    label="日志保留(天)"
                    value={policyDraft.logsDays}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, logsDays: v } : prev))
                    }
                    placeholder="30"
                  />
                  <Field
                    label="事件保留(天)"
                    value={policyDraft.eventsDays}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, eventsDays: v } : prev))
                    }
                    placeholder="30"
                  />
                  <Field
                    label="分析事件保留(天)"
                    value={policyDraft.trackEventsDays}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, trackEventsDays: v } : prev))
                    }
                    placeholder="0"
                  />
                  <Field
                    label="UTC 小时"
                    value={policyDraft.hourUTC}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, hourUTC: v } : prev))
                    }
                    placeholder="3"
                  />
                  <Field
                    label="UTC 分钟"
                    value={policyDraft.minuteUTC}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, minuteUTC: v } : prev))
                    }
                    placeholder="0"
                  />
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <button
                    className="rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
                    disabled={cleanupBusy}
                    onClick={async () => {
                      if (!policyDraft) return;
                      try {
                        setCleanupBusy(true);
                        setCleanupMsg("");
                        const saved = await upsertCleanupPolicy(settings, {
                          enabled: policyDraft.enabled,
                          logs_retention_days: Number(policyDraft.logsDays || "0"),
                          events_retention_days: Number(policyDraft.eventsDays || "0"),
                          track_events_retention_days: Number(policyDraft.trackEventsDays || "0"),
                          schedule_hour_utc: Number(policyDraft.hourUTC || "0"),
                          schedule_minute_utc: Number(policyDraft.minuteUTC || "0"),
                        });
                        setPolicy(saved);
                        const trackDays = saved.track_events_retention_days ?? 0;
                        const warn =
                          trackDays > 0 && trackDays < funnelDays
                            ? `；注意：当前漏斗时间范围=${funnelDays} 天，但分析事件保留=${trackDays} 天。建议把“分析事件保留(天)”调到 ≥ ${funnelDays}，或把漏斗时间范围调到 ≤ ${trackDays}。`
                            : "";
                        setCleanupMsg(
                          `已保存：enabled=${saved.enabled} logs=${saved.logs_retention_days}d events=${saved.events_retention_days}d track=${trackDays}d next=${saved.next_run_at ?? "-"}${warn}`,
                        );
                      } catch (e) {
                        setCleanupMsg(e instanceof Error ? e.message : String(e));
                      } finally {
                        setCleanupBusy(false);
                      }
                    }}
                  >
                    保存策略
                  </button>
                  <button
                    className="rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-900 disabled:opacity-60"
                    disabled={cleanupBusy || !policy?.enabled}
                    onClick={async () => {
                      try {
                        setCleanupBusy(true);
                        setCleanupMsg("");
                        const res = await runCleanupPolicy(settings);
                        setCleanupMsg(
                          `已清理：logs=${res.logs_deleted} (before ${res.logs_before || "-"}) events=${res.events_deleted} (before ${res.events_before || "-"}) track=${res.track_events_deleted} (before ${res.track_events_before || "-"})`,
                        );
                        setStorage(await getStorageEstimate(settings).catch(() => storage));
                        setPolicy(await getCleanupPolicy(settings).catch(() => policy));
                      } catch (e) {
                        setCleanupMsg(e instanceof Error ? e.message : String(e));
                      } finally {
                        setCleanupBusy(false);
                      }
                    }}
                  >
                    按策略立即清理
                  </button>
                </div>

                {policy ? (
                  <div className="text-xs text-zinc-500">
                    last: {policy.last_run_at ? new Date(policy.last_run_at).toLocaleString() : "-"}
                    {" / "}
                    next: {policy.next_run_at ? new Date(policy.next_run_at).toLocaleString() : "-"}
                  </div>
                ) : null}
              </div>
            )}
          </div>

          <div className="rounded-xl border border-zinc-900 bg-zinc-950 p-4">
            <div className="text-sm font-semibold">手动清理</div>
            <div className="mt-1 text-xs text-zinc-500">
              手动指定“早于某时间”的数据清理（会删除对应日志/事件）。
            </div>

            <div className="mt-4 space-y-3">
              <div>
                <div className="text-xs text-zinc-400">before（本地时间）</div>
                <input
                  type="datetime-local"
                  value={manualBeforeLocal}
                  onChange={(e) => setManualBeforeLocal(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
                />
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <button
                  className="rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-900 disabled:opacity-60"
                  disabled={cleanupBusy}
                  onClick={async () => {
                    try {
                      setCleanupBusy(true);
                      setCleanupMsg("");
                      const before = fromDateTimeLocalToRFC3339(manualBeforeLocal);
                      const res = await cleanupLogsBefore(settings, before);
                      setCleanupMsg(`已清理日志：deleted=${res.deleted} (before ${before})`);
                      setStorage(await getStorageEstimate(settings).catch(() => storage));
                    } catch (e) {
                      setCleanupMsg(e instanceof Error ? e.message : String(e));
                    } finally {
                      setCleanupBusy(false);
                    }
                  }}
                >
                  清理日志
                </button>
                <button
                  className="rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-900 disabled:opacity-60"
                  disabled={cleanupBusy}
                  onClick={async () => {
                    try {
                      setCleanupBusy(true);
                      setCleanupMsg("");
                      const before = fromDateTimeLocalToRFC3339(manualBeforeLocal);
                      const res = await cleanupEventsBefore(settings, before);
                      setCleanupMsg(`已清理事件：deleted=${res.deleted} (before ${before})`);
                      setStorage(await getStorageEstimate(settings).catch(() => storage));
                    } catch (e) {
                      setCleanupMsg(e instanceof Error ? e.message : String(e));
                    } finally {
                      setCleanupBusy(false);
                    }
                  }}
                >
                  清理事件
                </button>
              </div>
            </div>
          </div>
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

function toDateTimeLocal(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const yyyy = d.getFullYear();
  const mm = pad(d.getMonth() + 1);
  const dd = pad(d.getDate());
  const hh = pad(d.getHours());
  const mi = pad(d.getMinutes());
  return `${yyyy}-${mm}-${dd}T${hh}:${mi}`;
}

function fromDateTimeLocalToRFC3339(localValue: string): string {
  const raw = (localValue || "").trim();
  if (!raw) throw new Error("before required");
  const d = new Date(raw);
  if (!Number.isFinite(d.getTime())) throw new Error("invalid before");
  return d.toISOString();
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
