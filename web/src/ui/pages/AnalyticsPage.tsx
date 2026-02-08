import { useEffect, useMemo, useState } from "react";
import {
  getActiveSeries,
  getCleanupPolicy,
  getDistribution,
  getFunnel,
  getRetention,
  getTopEvents,
  type ActiveSeriesResponse,
  type CleanupPolicy,
  type DistributionResponse,
  type FunnelResponse,
  type RetentionResponse,
  type TopEventsResponse,
} from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { clampFunnelDays, loadFunnelDays, saveFunnelDays } from "../../lib/prefs";
import { Panel } from "../components/Panel";
import { Sparkline } from "../components/Sparkline";
import { useNavigate } from "react-router-dom";

export function AnalyticsPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [dau, setDau] = useState<ActiveSeriesResponse | null>(null);
  const [mau, setMau] = useState<ActiveSeriesResponse | null>(null);
  const [osDist, setOsDist] = useState<DistributionResponse | null>(null);
  const [countryDist, setCountryDist] = useState<DistributionResponse | null>(
    null,
  );
  const [operatorDist, setOperatorDist] = useState<DistributionResponse | null>(
    null,
  );
  const [retention, setRetention] = useState<RetentionResponse | null>(null);
  const [topEvents, setTopEvents] = useState<TopEventsResponse | null>(null);
  const [cleanupPolicy, setCleanupPolicy] = useState<CleanupPolicy | null>(null);
  const [funnelStepsText, setFunnelStepsText] = useState("signup,checkout,paid");
  const [funnelWithin, setFunnelWithin] = useState("24h");
  const [funnelDays, setFunnelDays] = useState(() => loadFunnelDays());
  const [funnel, setFunnel] = useState<FunnelResponse | null>(null);
  const [funnelBusy, setFunnelBusy] = useState(false);
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
        const [d, m, os, country, op, ret, top, cp] = await Promise.all([
          getActiveSeries(settings, { bucket: "day" }),
          getActiveSeries(settings, { bucket: "month" }),
          getDistribution(settings, { dim: "os", limit: 10 }),
          getDistribution(settings, { dim: "country", limit: 10 }),
          getDistribution(settings, { dim: "asn_org", limit: 10 }),
          getRetention(settings),
          getTopEvents(settings, { limit: 20 }),
          getCleanupPolicy(settings).catch(() => null),
        ]);
        if (cancelled) return;
        setDau(d);
        setMau(m);
        setOsDist(os);
        setCountryDist(country);
        setOperatorDist(op);
        setRetention(ret);
        setTopEvents(top);
        setCleanupPolicy(cp);
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
    <div className="space-y-4">
      <div>
        <div className="text-lg font-semibold">分析</div>
        <div className="mt-1 text-sm text-zinc-400">
          去重口径：优先 user.id，否则 device_id
        </div>
      </div>

      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Panel title="DAU（近 14 天）">
          {dau ? (
            <div className="space-y-3">
              <div className="flex items-end justify-between">
                <div className="text-2xl font-semibold">
                  {dau.series.at(-1)?.active ?? 0}
                </div>
                <div className="text-xs text-zinc-500">{dau.series.at(-1)?.bucket}</div>
              </div>
              <Sparkline values={dau.series.map((s) => s.active)} />
              <div className="grid grid-cols-2 gap-2 text-xs text-zinc-500 md:grid-cols-4">
                {dau.series.slice(-4).map((s) => (
                  <div key={s.bucket} className="rounded-lg border border-zinc-900 p-2">
                    <div>{s.bucket}</div>
                    <div className="mt-1 text-sm text-zinc-200">{s.active}</div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="text-sm text-zinc-500">加载中...</div>
          )}
        </Panel>

        <Panel title="MAU（近 6 个月）">
          {mau ? (
            <div className="space-y-3">
              <div className="flex items-end justify-between">
                <div className="text-2xl font-semibold">
                  {mau.series.at(-1)?.active ?? 0}
                </div>
                <div className="text-xs text-zinc-500">{mau.series.at(-1)?.bucket}</div>
              </div>
              <Sparkline values={mau.series.map((s) => s.active)} />
              <div className="grid grid-cols-2 gap-2 text-xs text-zinc-500 md:grid-cols-3">
                {mau.series.slice(-3).map((s) => (
                  <div key={s.bucket} className="rounded-lg border border-zinc-900 p-2">
                    <div>{s.bucket}</div>
                    <div className="mt-1 text-sm text-zinc-200">{s.active}</div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="text-sm text-zinc-500">加载中...</div>
          )}
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Panel title="终端系统（Top 10 / 近 7 天）">
          <DistTable data={osDist} emptyHint="暂无 OS 数据（需 SDK 上报 contexts.os 或 tags.device_id）" />
        </Panel>
        <Panel title="国家/地区（Top 10 / 近 7 天）">
          <DistTable data={countryDist} emptyHint="暂无国家分布（需要配置 GeoIP mmdb）" />
        </Panel>
        <Panel title="运营商/组织（Top 10 / 近 7 天）">
          <DistTable data={operatorDist} emptyHint="暂无运营商分布（需要配置 GeoIP ASN mmdb）" />
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Panel title="留存（近 14 天 / D1-D7-D30）">
          <RetentionTable data={retention} />
        </Panel>

        <Panel
          title="事件分析（自定义日志 message）"
          right={
            <div className="text-xs text-zinc-500">
              事件=日志 message；用户=distinct_id
            </div>
          }
        >
          <div className="space-y-4">
            <TopEventsTable data={topEvents} />

            <div className="rounded-lg border border-zinc-900 p-3">
              <div className="text-sm font-semibold">漏斗</div>
              <div className="mt-1 grid grid-cols-1 gap-2 md:grid-cols-4">
                <div>
                  <div className="text-xs text-zinc-400">steps（逗号分隔）</div>
                  <input
                    value={funnelStepsText}
                    onChange={(e) => setFunnelStepsText(e.target.value)}
                    placeholder="signup,checkout,paid"
                    className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
                  />
                </div>
                <div>
                  <div className="text-xs text-zinc-400">within（可选）</div>
                  <input
                    value={funnelWithin}
                    onChange={(e) => setFunnelWithin(e.target.value)}
                    placeholder="24h"
                    className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
                  />
                </div>
                <div>
                  <div className="text-xs text-zinc-400">时间范围（天，默认 7）</div>
                  <input
                    value={String(funnelDays)}
                    onChange={(e) => {
                      const next = clampFunnelDays(Number(e.target.value || "7"));
                      setFunnelDays(next);
                      saveFunnelDays(next);
                    }}
                    placeholder="7"
                    className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
                  />
                </div>
                <div className="flex items-end">
                  <button
                    className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
                    onClick={async () => {
                      try {
                        setFunnelBusy(true);
                        setErr("");
                        const steps = funnelStepsText
                          .split(",")
                          .map((s) => s.trim())
                          .filter(Boolean);
                        if (!settings.token || !settings.projectId) return;

                        const rangeDays = clampFunnelDays(funnelDays);
                        const trackRetention = cleanupPolicy?.track_events_retention_days ?? 0;
                        if (trackRetention > 0 && rangeDays > trackRetention) {
                          setErr(
                            `漏斗时间范围=${rangeDays} 天，但分析事件保留=${trackRetention} 天；会导致漏斗明细不足无法保证精确。请在“概览-自动清理”把“分析事件保留(天)”调到 ≥ ${rangeDays}，或把漏斗时间范围调到 ≤ ${trackRetention}。`,
                          );
                          return;
                        }

                        const end = new Date();
                        const start = new Date(end.getTime() - (rangeDays - 1) * 24 * 3600 * 1000);
                        const res = await getFunnel(settings, {
                          steps,
                          start: start.toISOString(),
                          end: end.toISOString(),
                          within: funnelWithin.trim() || undefined,
                        });
                        setFunnel(res);
                      } catch (e) {
                        setErr(e instanceof Error ? e.message : String(e));
                      } finally {
                        setFunnelBusy(false);
                      }
                    }}
                    disabled={funnelBusy}
                  >
                    {funnelBusy ? "计算中..." : "计算"}
                  </button>
                </div>
              </div>
              {cleanupPolicy?.track_events_retention_days ? (
                <div className="mt-2 text-xs text-zinc-500">
                  分析事件保留：{cleanupPolicy.track_events_retention_days} 天（漏斗依赖该明细；过短会无法精确计算）
                </div>
              ) : (
                <div className="mt-2 text-xs text-zinc-500">
                  分析事件保留：未配置（0=不清理）；漏斗依赖分析事件明细
                </div>
              )}

              <FunnelTable data={funnel} />
            </div>
          </div>
        </Panel>
      </div>
    </div>
  );
}

function DistTable(props: { data: DistributionResponse | null; emptyHint: string }) {
  const items = props.data?.items ?? [];
  if (!props.data) {
    return <div className="text-sm text-zinc-500">加载中...</div>;
  }
  if (items.length === 0) {
    return <div className="text-sm text-zinc-500">{props.emptyHint}</div>;
  }
  const max = Math.max(...items.map((i) => i.count), 1);
  return (
    <div className="space-y-3">
      <div className="text-xs text-zinc-500">
        {props.data.start.slice(0, 10)} ~ {props.data.end.slice(0, 10)}
      </div>
      <div className="space-y-2">
        {items.map((it) => (
          <div key={it.key} className="flex items-center gap-3">
            <div className="w-32 shrink-0 truncate text-sm text-zinc-200" title={it.key}>
              {it.key}
            </div>
            <div className="h-2 flex-1 overflow-hidden rounded bg-zinc-900">
              <div
                className="h-2 rounded bg-indigo-500/70"
                style={{ width: `${Math.round((it.count / max) * 100)}%` }}
              />
            </div>
            <div className="w-12 text-right font-mono text-xs text-zinc-400">{it.count}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

function RetentionTable(props: { data: RetentionResponse | null }) {
  if (!props.data) {
    return <div className="text-sm text-zinc-500">加载中...</div>;
  }
  const rows = props.data.rows ?? [];
  if (rows.length === 0) {
    return <div className="text-sm text-zinc-500">暂无数据</div>;
  }
  const days = props.data.days ?? [];
  const last = rows.slice(-14);
  return (
    <div className="space-y-3">
      <div className="text-xs text-zinc-500">
        {props.data.start.slice(0, 10)} ~ {props.data.end.slice(0, 10)}
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-zinc-500">
            <tr>
              <th className="py-2 pr-4">Cohort</th>
              <th className="py-2 pr-4">Size</th>
              {days.map((d) => (
                <th key={d} className="py-2 pr-4">
                  D{d}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-900">
            {last.map((r) => (
              <tr key={r.cohort} className="hover:bg-zinc-900/40">
                <td className="py-2 pr-4 text-zinc-300">{r.cohort}</td>
                <td className="py-2 pr-4 font-mono text-xs text-zinc-300">
                  {r.cohort_size}
                </td>
                {days.map((d) => {
                  const p = r.points.find((x) => x.day === d);
                  const pct = Math.round(((p?.rate ?? 0) * 1000)) / 10;
                  return (
                    <td key={d} className="py-2 pr-4 font-mono text-xs text-zinc-200">
                      {pct}%
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function TopEventsTable(props: { data: TopEventsResponse | null }) {
  if (!props.data) {
    return <div className="text-sm text-zinc-500">加载中...</div>;
  }
  const items = props.data.items ?? [];
  if (items.length === 0) {
    return <div className="text-sm text-zinc-500">暂无事件（先通过 /logs/ 上报）</div>;
  }
  return (
    <div className="space-y-2">
      <div className="text-xs text-zinc-500">
        Top {items.length}（{props.data.start.slice(0, 10)} ~ {props.data.end.slice(0, 10)}）
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-zinc-500">
            <tr>
              <th className="py-2 pr-4">事件</th>
              <th className="py-2 pr-4">次数</th>
              <th className="py-2 pr-4">用户</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-900">
            {items.slice(0, 10).map((it) => (
              <tr key={it.name} className="hover:bg-zinc-900/40">
                <td className="py-2 pr-4 text-zinc-100">
                  <span className="block max-w-[28rem] truncate" title={it.name}>
                    {it.name}
                  </span>
                </td>
                <td className="py-2 pr-4 font-mono text-xs text-zinc-300">{it.events}</td>
                <td className="py-2 pr-4 font-mono text-xs text-zinc-300">{it.users}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function FunnelTable(props: { data: FunnelResponse | null }) {
  if (!props.data) {
    return <div className="mt-3 text-sm text-zinc-500">尚未计算</div>;
  }
  const steps = props.data.steps ?? [];
  if (steps.length === 0) {
    return <div className="mt-3 text-sm text-zinc-500">暂无数据</div>;
  }
  return (
    <div className="mt-3 overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs text-zinc-500">
          <tr>
            <th className="py-2 pr-4">步骤</th>
            <th className="py-2 pr-4">用户</th>
            <th className="py-2 pr-4">转化</th>
            <th className="py-2 pr-4">流失</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-900">
          {steps.map((s, idx) => {
            const pct = Math.round((s.conversion * 1000)) / 10;
            return (
              <tr key={`${idx}-${s.name}`} className="hover:bg-zinc-900/40">
                <td className="py-2 pr-4 text-zinc-100">{s.name}</td>
                <td className="py-2 pr-4 font-mono text-xs text-zinc-300">{s.users}</td>
                <td className="py-2 pr-4 font-mono text-xs text-zinc-200">
                  {idx === 0 ? "100%" : `${pct}%`}
                </td>
                <td className="py-2 pr-4 font-mono text-xs text-zinc-300">
                  {idx === 0 ? "-" : s.dropoff}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
