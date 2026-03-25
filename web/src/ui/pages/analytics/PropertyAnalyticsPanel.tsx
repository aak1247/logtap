import { useEffect, useState } from "react";
import { Panel } from "../../components/Panel";
import { Sparkline } from "../../components/Sparkline";
import {
  postCustomAnalytics,
  createAnalysisView,
  listAnalysisViews,
  listPropertyDefinitions,
  type AnalysisView,
  type ApiSettings,
  type CustomAnalyticsResponse,
  type CustomAnalyticsSeries,
  type CustomAnalyticsSeriesPoint,
  type PropertyDefinition,
} from "../../../lib/api";

export type PropertyAnalyticsPanelProps = {
  // ApiSettings 从上层 AnalyticsPage 传入（已包含 token/projectId）
  settings: ApiSettings;
};

export function PropertyAnalyticsPanel(props: PropertyAnalyticsPanelProps) {
  const { settings } = props;
  const [propertyKey, setPropertyKey] = useState("");
  const [days, setDays] = useState(7);
  const [withTime, setWithTime] = useState(true);
  const [eventFilterText, setEventFilterText] = useState("");
  const [metric, setMetric] = useState<"count_events" | "count_users">(
    "count_events",
  );
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>("");
  const [result, setResult] = useState<CustomAnalyticsResponse | null>(null);

  const [propDefs, setPropDefs] = useState<PropertyDefinition[] | null>(null);
  const [defsErr, setDefsErr] = useState("");

  const [showSave, setShowSave] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [saveDescription, setSaveDescription] = useState("");
  const [saveBusy, setSaveBusy] = useState(false);

  const [showLoad, setShowLoad] = useState(false);
  const [views, setViews] = useState<AnalysisView[] | null>(null);
  const [viewsBusy, setViewsBusy] = useState(false);
  const [chartType, setChartType] = useState<"bar" | "pie">("bar");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setDefsErr("");
        const res = await listPropertyDefinitions(settings, { status: "active" });
        if (!cancelled) setPropDefs(res.items ?? []);
      } catch (e) {
        if (!cancelled) setDefsErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId, settings.token]);

  const buildRequestBody = (key: string) => {
    const trimmedEvents = eventFilterText
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    const events = Array.from(new Set(trimmedEvents));

    const now = new Date();
    const end = now.toISOString();
    const dayCount = Number.isFinite(days) && days > 0 ? days : 7;
    const startDate = new Date(
      now.getTime() - (dayCount - 1) * 24 * 60 * 60 * 1000,
    );
    const start = startDate.toISOString();

    const groupBy: string[] = [];
    if (withTime) {
      groupBy.push("time");
    }
    groupBy.push(`property:${key}`);

    return {
      analysis_type: "property" as const,
      time_range: {
        start,
        end,
        granularity: "day" as const,
      },
      target: {
        events,
        property: key,
      },
      metric: {
        type: metric,
      },
      group_by: groupBy,
      filter: {
        events,
        properties: {},
      },
    };
  };

  const runAnalysis = async (body: unknown) => {
    if (!settings.token || !settings.projectId) return;
    setBusy(true);
    setErr("");
    try {
      const res = await postCustomAnalytics(settings, body);
      setResult(res);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleRun = async () => {
    if (!settings.token || !settings.projectId) return;
    const key = propertyKey.trim();
    if (!key) {
      setErr("请先填写要分析的属性 key");
      return;
    }
    const body = buildRequestBody(key);
    await runAnalysis(body);
  };

  const handleOpenSave = () => {
    if (!settings.token || !settings.projectId) return;
    if (!saveName) {
      const key = propertyKey.trim();
      const label = key || "属性";
      setSaveName(`属性分析 - ${label}`);
    }
    setShowSave(true);
  };

  const handleSave = async () => {
    if (!settings.token || !settings.projectId) return;
    const name = saveName.trim();
    if (!name) {
      setErr("报表名称不能为空");
      return;
    }
    const key = propertyKey.trim();
    if (!key) {
      setErr("请先填写要分析的属性 key，再保存报表");
      return;
    }
    const body = buildRequestBody(key);
    const query = {
      ...body,
      presentation: {
        chart_type: chartType,
      },
    };
    setSaveBusy(true);
    setErr("");
    try {
      await createAnalysisView(settings, {
        name,
        description: saveDescription.trim(),
        analysis_type: "property",
        query,
      });
      setShowSave(false);
      if (showLoad) {
        await loadViews();
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setSaveBusy(false);
    }
  };


  const loadViews = async () => {
    if (!settings.token || !settings.projectId) return;
    setViewsBusy(true);
    setErr("");
    try {
      const res = await listAnalysisViews(settings, {
        analysis_type: "property",
      });
      setViews(res.items ?? []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setViewsBusy(false);
    }
  };

  const handleOpenLoad = async () => {
    if (!settings.token || !settings.projectId) return;
    setShowLoad(true);
    await loadViews();
  };

  const applyView = async (view: AnalysisView) => {
    if (!settings.token || !settings.projectId) return;
    const q = view.query as any;
    if (!q || typeof q !== "object") {
      setErr("报表配置格式不正确");
      return;
    }
    if (typeof q.analysis_type === "string" && q.analysis_type !== "property") {
      setErr("该报表不是属性分析类型");
      return;
    }

    try {
      const tr = q.time_range ?? {};
      if (typeof tr.start === "string" && typeof tr.end === "string") {
        const start = new Date(tr.start);
        const end = new Date(tr.end);
        if (!Number.isNaN(start.getTime()) && !Number.isNaN(end.getTime())) {
          const diffMs = Math.max(0, end.getTime() - start.getTime());
          const diffDays = Math.floor(diffMs / (24 * 60 * 60 * 1000)) + 1;
          const clamped = Math.min(Math.max(diffDays, 1), 180);
          setDays(clamped);
        }
      }
      const target = q.target ?? {};
      if (typeof target.property === "string") {
        setPropertyKey(target.property);
      }
      const ev = q.target?.events ?? q.filter?.events;
      if (Array.isArray(ev)) {
        setEventFilterText(ev.join(" "));
      }
      const m = q.metric ?? {};
      if (typeof m.type === "string") {
        const t = m.type === "count_users" ? "count_users" : "count_events";
        setMetric(t);
      }
      const gb = Array.isArray(q.group_by) ? q.group_by : [];
      setWithTime(gb.includes("time"));
      const pres = q.presentation ?? {};
      const ct = typeof pres.chart_type === "string" ? pres.chart_type : "";
      if (ct === "bar" || ct === "pie") {
        setChartType(ct);
      } else {
        setChartType("bar");
      }
    } catch {
      // ignore restoration errors
    }

    await runAnalysis(q);
    setShowLoad(false);
  };

  return (
    <div className="space-y-4">
      <Panel
        title="属性分析"
        right={
          <div className="text-xs text-zinc-500">
            按单个属性值维度统计事件次数或去重用户数
          </div>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
            <div className="md:col-span-2">
              <div className="text-xs text-zinc-400">属性 key</div>
              <input
                value={propertyKey}
                onChange={(e) => setPropertyKey(e.target.value)}
                placeholder="例如 plan 或 device.os"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
              <div className="mt-1 text-xs text-zinc-500">
                当前仅支持单个属性；属性值来源于日志 fields JSON
              </div>
              {propDefs && propDefs.length > 0 ? (
                <div className="mt-2 text-xs text-zinc-400">
                  <div className="mb-1">从属性定义选择：</div>
                  <select
                    value={propertyKey}
                    onChange={(e) => setPropertyKey(e.target.value)}
                    className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                  >
                    <option value="">请选择属性...</option>
                    {propDefs.map((d) => (
                      <option key={d.id} value={d.key}>
                        {(d.display_name || d.key) + (d.type ? ` (${d.type})` : "")}
                      </option>
                    ))}
                  </select>
                </div>
              ) : null}
            </div>
            <div>
              <div className="text-xs text-zinc-400">时间范围（天）</div>
              <input
                type="number"
                min={1}
                max={180}
                value={days}
                onChange={(e) => {
                  const v = Number(e.target.value || "7");
                  if (!Number.isFinite(v) || v <= 0) {
                    setDays(7);
                  } else if (v > 180) {
                    setDays(180);
                  } else {
                    setDays(v);
                  }
                }}
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
            </div>
            <div>
              <div className="text-xs text-zinc-400">指标</div>
              <div className="mt-1 space-y-1 text-sm text-zinc-200">
                <label className="flex items-center gap-2">
                  <input
                    type="radio"
                    className="h-3 w-3"
                    checked={metric === "count_events"}
                    onChange={() => setMetric("count_events")}
                  />
                  <span>事件次数</span>
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="radio"
                    className="h-3 w-3"
                    checked={metric === "count_users"}
                    onChange={() => setMetric("count_users")}
                  />
                  <span>去重用户数</span>
                </label>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
            <div className="md:col-span-2">
              <div className="text-xs text-zinc-400">事件过滤（可选）</div>
              <input
                value={eventFilterText}
                onChange={(e) => setEventFilterText(e.target.value)}
                placeholder="例如 signup checkout"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
              <div className="mt-1 text-xs text-zinc-500">
                留空表示统计所有事件；多个事件使用空格或逗号分隔
              </div>
            </div>
            <label className="flex items-center gap-2 text-sm text-zinc-200">
              <input
                type="checkbox"
                className="h-3 w-3"
                checked={withTime}
                onChange={(e) => setWithTime(e.target.checked)}
              />
              <span>按时间查看趋势（不勾选则仅看整体分布）</span>
            </label>
            <div className="flex items-end">
              <button
                type="button"
                disabled={busy}
                className="btn btn-md btn-primary w-full disabled:cursor-not-allowed disabled:opacity-60"
                onClick={handleRun}
              >
                {busy ? "分析中..." : "运行分析"}
              </button>
            </div>
          </div>

          {err ? (
            <div className="rounded-md border border-red-900/60 bg-red-950/40 p-2 text-xs text-red-200">
              {err}
            </div>
          ) : null}

          {defsErr && !err ? (
            <div className="rounded-md border border-yellow-900/60 bg-yellow-950/40 p-2 text-xs text-yellow-200">
              属性定义加载失败：{defsErr}
            </div>
          ) : null}
        </div>
      </Panel>

      <div className="flex items-center justify-end gap-2">
        <button
          type="button"
          className="btn btn-sm btn-secondary"
          onClick={handleOpenSave}
          disabled={busy}
        >
          保存为报表
        </button>
        <button
          type="button"
          className="btn btn-sm btn-secondary"
          onClick={handleOpenLoad}
          disabled={busy}
        >
          打开报表
        </button>
      </div>

      {showSave ? (
        <Panel title="保存属性分析报表">
          <div className="space-y-3 text-sm">
            <div>
              <div className="text-xs text-zinc-400">名称</div>
              <input
                value={saveName}
                onChange={(e) => setSaveName(e.target.value)}
                placeholder="例如 属性分析 - plan"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
            </div>
            <div>
              <div className="text-xs text-zinc-400">描述（可选）</div>
              <input
                value={saveDescription}
                onChange={(e) => setSaveDescription(e.target.value)}
                placeholder="例如 最近 7 天 plan 属性值分布"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
            </div>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => setShowSave(false)}
                disabled={saveBusy}
              >
                取消
              </button>
              <button
                type="button"
                className="btn btn-sm btn-primary"
                onClick={handleSave}
                disabled={saveBusy}
              >
                {saveBusy ? "保存中..." : "保存"}
              </button>
            </div>
          </div>
        </Panel>
      ) : null}

      {showLoad ? (
        <Panel title="属性分析报表列表">
          <div className="space-y-3 text-sm">
            {viewsBusy ? (
              <div className="text-zinc-500">加载中...</div>
            ) : !views || views.length === 0 ? (
              <div className="text-zinc-500">暂无报表，可以先保存一条。</div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead className="text-xs text-zinc-500">
                    <tr>
                      <th className="py-2 pr-4">名称</th>
                      <th className="py-2 pr-4">描述</th>
                      <th className="py-2 pr-4">更新时间</th>
                      <th className="py-2 pr-4">操作</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-900">
                    {views.map((v) => (
                      <tr key={v.id} className="hover:bg-zinc-900/40">
                        <td className="py-2 pr-4 text-zinc-100">
                          <span
                            className="block max-w-[20rem] truncate"
                            title={v.name}
                          >
                            {v.name}
                          </span>
                        </td>
                        <td className="py-2 pr-4 text-xs text-zinc-400">
                          <span
                            className="block max-w-[24rem] truncate"
                            title={v.description}
                          >
                            {v.description}
                          </span>
                        </td>
                        <td className="py-2 pr-4 text-xs text-zinc-500">
                          {v.updated_at?.slice(0, 19).replace("T", " ")}
                        </td>
                        <td className="py-2 pr-4">
                          <button
                            type="button"
                            className="btn btn-xs btn-primary"
                            onClick={() => void applyView(v)}
                            disabled={busy}
                          >
                            应用
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <div className="flex justify-end">
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => setShowLoad(false)}
                disabled={busy}
              >
                关闭
              </button>
            </div>
          </div>
        </Panel>
      ) : null}

      <PropertyAnalyticsResult
        result={result}
        propDefs={propDefs}
        chartType={chartType}
        onChartTypeChange={setChartType}
      />
    </div>
  );
}

function PropertyAnalyticsResult(props: { result: CustomAnalyticsResponse | null; propDefs: PropertyDefinition[] | null; chartType: "bar" | "pie"; onChartTypeChange: (v: "bar" | "pie") => void }) {
  const { result, propDefs, chartType, onChartTypeChange } = props;

  if (!result) {
    return (
      <Panel title="结果">
        <div className="text-sm text-zinc-500">尚未执行分析，可以先在上方配置并运行。</div>
      </Panel>
    );
  }
  const series = result.series ?? [];
  const propertyKey = result.property_key || "";
  const propDef = propDefs?.find((d) => d.key === propertyKey);
  const propLabel = propDef
    ? (propDef.display_name && propDef.display_name !== propDef.key
        ? `${propDef.display_name} (${propDef.key})`
        : propDef.key)
    : propertyKey;
  const propDesc = propDef?.description || "";
  if (series.length === 0) {
    return (
      <Panel title="结果">
        <div className="text-sm text-zinc-500">暂无数据（当前条件下没有命中记录）。</div>
      </Panel>
    );
  }

  const withTime = result.group_by.includes("time");

  if (!withTime) {
    const max = Math.max(...series.map((s) => s.total), 1);
    const total = series.reduce((acc, s) => acc + s.total, 0) || 1;

    return (
      <Panel
        title="属性值分布"
        right={
          <div className="flex items-center gap-4 text-xs text-zinc-500">
            <div>
              属性：{propLabel || "(未指定)"} / 时间范围：{result.start.slice(0, 10)} ~ {result.end.slice(0, 10)}
              {propDesc ? `（${propDesc}）` : ""}
            </div>
            <div className="flex items-center gap-2">
              <span>图表类型</span>
              <div className="inline-flex overflow-hidden rounded border border-zinc-700 text-[11px]">
                <button
                  type="button"
                  className={
                    "px-2 py-0.5 " +
                    (chartType === "bar"
                      ? "bg-zinc-800 text-zinc-50"
                      : "bg-zinc-900 text-zinc-400")
                  }
                  onClick={() => onChartTypeChange("bar")}
                >
                  条形
                </button>
                <button
                  type="button"
                  className={
                    "px-2 py-0.5 border-l border-zinc-700 " +
                    (chartType === "pie"
                      ? "bg-zinc-800 text-zinc-50"
                      : "bg-zinc-900 text-zinc-400")
                  }
                  onClick={() => onChartTypeChange("pie")}
                >
                  饼图
                </button>
              </div>
            </div>
          </div>
        }
      >
        <div className="space-y-4">
          {chartType === "pie" ? (
            <PropertyPie series={series} total={total} />
          ) : (
            <div className="space-y-2">
              {series.map((s: CustomAnalyticsSeries) => (
                <div key={s.name} className="flex items-center gap-3">
                  <div
                    className="w-40 shrink-0 truncate text-sm text-zinc-200"
                    title={s.name}
                  >
                    {s.name || "(空)"}
                  </div>
                  <div className="h-2 flex-1 overflow-hidden rounded bg-zinc-900">
                    <div
                      className="h-2 rounded bg-indigo-500/70"
                      style={{ width: `${Math.round((s.total / max) * 100)}%` }}
                    />
                  </div>
                  <div className="w-24 text-right text-xs text-zinc-400">
                    <span className="font-mono mr-1">{s.total}</span>
                    <span>({Math.round((s.total / total) * 100)}%)</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </Panel>
    );
  }

  return (
    <Panel
      title="属性堆积趋势"
      right={
        <div className="text-xs text-zinc-500">
          属性：{propLabel || "(未指定)"} / 时间范围：{result.start.slice(0, 10)} ~ {result.end.slice(0, 10)}（粒度：
          {result.granularity}）{propDesc ? `（${propDesc}）` : ""}
        </div>
      }
    >
      <div className="space-y-4">
        <div className="space-y-3">
          {series.map((s: CustomAnalyticsSeries) => (
            <PropertySeriesSparkline key={s.name} series={s} />
          ))}
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-zinc-500">
              <tr>
                <th className="py-2 pr-4">属性值</th>
                <th className="py-2 pr-4">Total</th>
                <th className="py-2 pr-4">Last Point</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {series.map((s: CustomAnalyticsSeries) => {
                const last = s.points.at(-1);
                return (
                  <tr key={s.name} className="hover:bg-zinc-900/40">
                    <td className="py-2 pr-4 text-zinc-100">
                      <span className="block max-w-[28rem] truncate" title={s.name}>
                        {s.name || "(空)"}
                      </span>
                    </td>
                    <td className="py-2 pr-4 font-mono text-xs text-zinc-300">
                      {s.total}
                    </td>
                    <td className="py-2 pr-4 font-mono text-xs text-zinc-400">
                      {last ? `${last.time}: ${last.value}` : "-"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </Panel>
  );
}

function PropertyPie(props: { series: CustomAnalyticsSeries[]; total: number }) {
  const { series, total } = props;
  const size = 180;
  const radius = size / 2 - 4;
  let angleStart = 0;

  const slices = series.map((s) => {
    const value = s.total;
    const fraction = value / (total || 1);
    const angle = fraction * Math.PI * 2;
    const angleEnd = angleStart + angle;
    const largeArc = angle > Math.PI ? 1 : 0;
    const x1 = radius * Math.cos(angleStart);
    const y1 = radius * Math.sin(angleStart);
    const x2 = radius * Math.cos(angleEnd);
    const y2 = radius * Math.sin(angleEnd);
    const path = `M 0 0 L ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${x2} ${y2} Z`;
    const midAngle = angleStart + angle / 2;
    angleStart = angleEnd;
    return { s, path, midAngle };
  });

  const colors = [
    "#6366f1",
    "#22c55e",
    "#f97316",
    "#e11d48",
    "#a855f7",
    "#0ea5e9",
    "#facc15",
  ];

  return (
    <div className="flex flex-wrap items-center gap-4">
      <svg
        width={size}
        height={size}
        viewBox={`${-size / 2} ${-size / 2} ${size} ${size}`}
        className="shrink-0"
      >
        {slices.map((slice, idx) => (
          <path
            key={slice.s.name + idx}
            d={slice.path}
            fill={colors[idx % colors.length]}
            fillOpacity={0.85}
          />
        ))}
      </svg>
      <div className="space-y-1 text-xs text-zinc-300">
        {series.map((s, idx) => (
          <div key={s.name + idx} className="flex items-center gap-2">
            <span
              className="inline-block h-2 w-2 rounded-sm"
              style={{ backgroundColor: colors[idx % colors.length] }}
            />
            <span className="max-w-[12rem] truncate" title={s.name}>
              {s.name || "(空)"}
            </span>
            <span className="font-mono text-zinc-400">
              {s.total} ({Math.round((s.total / (total || 1)) * 100)}%)
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function PropertySeriesSparkline(props: { series: CustomAnalyticsSeries }) {
  const { series } = props;
  const values = series.points.map(
    (p: CustomAnalyticsSeriesPoint) => p.value,
  );
  const label = series.name || series.dimensions.property || "(空)";

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <div className="truncate text-sm text-zinc-200" title={label}>
          {label}
        </div>
        <div className="text-xs text-zinc-500">
          {series.points.length} 点；总计 {series.total}
        </div>
      </div>
      <Sparkline values={values} />
    </div>
  );
}

