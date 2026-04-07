import { useEffect, useState } from "react";
import { Panel } from "../../components/Panel";
import { Sparkline } from "../../components/Sparkline";
import { TimeRangePicker } from "../../components/DateTimePicker";
import {
  postCustomAnalytics,
  createAnalysisView,
  listAnalysisViews,
  listEventDefinitions,
  type AnalysisView,
  type ApiSettings,
  type CustomAnalyticsResponse,
  type CustomAnalyticsSeries,
  type CustomAnalyticsSeriesPoint,
  type EventDefinition,
} from "../../../lib/api";

export type EventAnalyticsPanelProps = {
  settings: ApiSettings;
};

export function EventAnalyticsPanel(props: EventAnalyticsPanelProps) {
  const { settings } = props;
  const [days, setDays] = useState(7);
  const [rangeStart, setRangeStart] = useState(() => buildRangeFromDays(7).start);
  const [rangeEnd, setRangeEnd] = useState(() => buildRangeFromDays(7).end);
  const [eventNamesText, setEventNamesText] = useState("");
  const [metric, setMetric] = useState<"count_events" | "count_users">(
    "count_events",
  );
  const [propertyKey, setPropertyKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>("");
  const [result, setResult] = useState<CustomAnalyticsResponse | null>(null);

  const [eventDefs, setEventDefs] = useState<EventDefinition[] | null>(null);
  const [defsErr, setDefsErr] = useState("");

  const [showSave, setShowSave] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [saveDescription, setSaveDescription] = useState("");
  const [saveBusy, setSaveBusy] = useState(false);

  const [showLoad, setShowLoad] = useState(false);
  const [views, setViews] = useState<AnalysisView[] | null>(null);
  const [viewsBusy, setViewsBusy] = useState(false);
  const [chartType, setChartType] = useState<"line" | "column">("line");

  const selectedEvents = parseEventNames(eventNamesText);

  const toggleEvent = (name: string) => {
    const current = parseEventNames(eventNamesText);
    let next: string[];
    if (current.includes(name)) {
      next = current.filter((n) => n !== name);
    } else {
      next = [...current, name];
    }
    setEventNamesText(next.join(" "));
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setDefsErr("");
        const res = await listEventDefinitions(settings, { status: "active" });
        if (!cancelled) setEventDefs(res.items ?? []);
      } catch (e) {
        if (!cancelled) setDefsErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId, settings.token]);

  const buildRequestBody = () => {
    const events = Array.from(new Set(parseEventNames(eventNamesText)));

    const dayCount = Number.isFinite(days) && days > 0 ? days : 7;
    const fallback = buildRangeFromDays(dayCount);
    const startDate = new Date(rangeStart);
    const endDate = new Date(rangeEnd);
    const start = Number.isNaN(startDate.getTime())
      ? fallback.start
      : startDate.toISOString();
    const end = Number.isNaN(endDate.getTime()) ? fallback.end : endDate.toISOString();

    const groupBy: string[] = ["time"];
    const propKey = propertyKey.trim();
    if (propKey) {
      groupBy.push(`property:${propKey}`);
    } else if (events.length > 1) {
      groupBy.push("event");
    }

    return {
      analysis_type: "event" as const,
      time_range: {
        start,
        end,
        granularity: "day" as const,
      },
      target: {
        events,
        property: propKey,
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
    const body = buildRequestBody();
    await runAnalysis(body);
  };

  const handleOpenSave = () => {
    if (!settings.token || !settings.projectId) return;
    if (!saveName) {
      const firstEvent = eventNamesText
        .split(/[\s,]+/)
        .map((s) => s.trim())
        .filter(Boolean)[0];
      const parts: string[] = [];
      if (firstEvent) parts.push(firstEvent);
      if (propertyKey.trim()) parts.push(`by ${propertyKey.trim()}`);
      const defaultName = parts.length
        ? `事件分析 - ${parts.join(" ")}`
        : "事件分析报表";
      setSaveName(defaultName);
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
    const body = buildRequestBody();
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
        analysis_type: "event",
        query,
      });
      setShowSave(false);
      // 如果当前已打开列表，刷新一次
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
      const res = await listAnalysisViews(settings, { analysis_type: "event" });
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
    if (typeof q.analysis_type === "string" && q.analysis_type !== "event") {
      setErr("该报表不是事件分析类型");
      return;
    }

    try {
      const tr = q.time_range ?? {};
      if (typeof tr.start === "string" && typeof tr.end === "string") {
        const start = new Date(tr.start);
        const end = new Date(tr.end);
        if (!Number.isNaN(start.getTime()) && !Number.isNaN(end.getTime())) {
          setRangeStart(start.toISOString());
          setRangeEnd(end.toISOString());
          const diffMs = Math.max(0, end.getTime() - start.getTime());
          const diffDays = Math.floor(diffMs / (24 * 60 * 60 * 1000)) + 1;
          const clamped = Math.min(Math.max(diffDays, 1), 180);
          setDays(clamped);
        }
      }
      const target = q.target ?? {};
      if (Array.isArray(target.events)) {
        setEventNamesText(target.events.join(" "));
      }
      if (typeof target.property === "string") {
        setPropertyKey(target.property);
      }
      const m = q.metric ?? {};
      if (typeof m.type === "string") {
        const t = m.type === "count_users" ? "count_users" : "count_events";
        setMetric(t);
      }
      const pres = q.presentation ?? {};
      const ct = typeof pres.chart_type === "string" ? pres.chart_type : "";
      if (ct === "line" || ct === "column") {
        setChartType(ct);
      } else {
        setChartType("line");
      }
    } catch {
      // ignore state restoration errors
    }

    await runAnalysis(q);
    setShowLoad(false);
  };

  return (
    <div className="space-y-4">
      <Panel
        title="事件分析"
        right={
          <div className="text-xs text-zinc-500">
            统计日志 level="event" 的 message；用户去重口径与基础分析一致
          </div>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
            <div>
              <TimeRangePicker
                label={`时间范围（${days} 天）`}
                start={rangeStart}
                end={rangeEnd}
                onStartChange={(nextStart) => {
                  setRangeStart(nextStart);
                  const nextDays = Math.min(
                    Math.max(getRangeDays(nextStart, rangeEnd, days), 1),
                    180,
                  );
                  setDays(nextDays);
                }}
                onEndChange={(nextEnd) => {
                  setRangeEnd(nextEnd);
                  const nextDays = Math.min(
                    Math.max(getRangeDays(rangeStart, nextEnd, days), 1),
                    180,
                  );
                  setDays(nextDays);
                }}
                onRangePresetChange={(nextStart, nextEnd) => {
                  const nextDays = Math.min(
                    Math.max(getRangeDays(nextStart, nextEnd, days), 1),
                    180,
                  );
                  setDays(nextDays);
                }}
              />
            </div>
            <div className="md:col-span-2">
              <div className="text-xs text-zinc-400">
                事件（可选，多事件使用逗号或空格分隔）
              </div>
              <input
                value={eventNamesText}
                onChange={(e) => setEventNamesText(e.target.value)}
                placeholder="signup checkout paid"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
              <div className="mt-1 text-xs text-zinc-500">
                留空表示统计所有事件；多个事件将按事件拆分绘制多条曲线
              </div>
              {eventDefs && eventDefs.length > 0 ? (
                <div className="mt-2 text-xs text-zinc-400">
                  <div className="mb-1">从事件定义选择：</div>
                  <div className="flex flex-wrap gap-1">
                    {eventDefs.map((d) => {
                      const name = d.name;
                      const label =
                        d.display_name && d.display_name !== d.name
                          ? `${d.display_name} (${d.name})`
                          : d.name;
                      const selected = selectedEvents.includes(name);
                      return (
                        <button
                          key={d.id}
                          type="button"
                          className={
                            "rounded-full border px-2 py-0.5 text-xs " +
                            (selected
                              ? "border-indigo-500 bg-indigo-500/20 text-indigo-100"
                              : "border-zinc-700 bg-zinc-950 text-zinc-300 hover:bg-zinc-900")
                          }
                          onClick={() => toggleEvent(name)}
                        >
                          {label}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ) : null}
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
              <div className="text-xs text-zinc-400">按属性拆分（可选）</div>
              <input
                value={propertyKey}
                onChange={(e) => setPropertyKey(e.target.value)}
                placeholder="例如 plan 或 device.os"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
              <div className="mt-1 text-xs text-zinc-500">
                仅支持单个属性；填写后将按属性值拆分多条曲线
              </div>
            </div>
            <div className="flex items-end gap-2">
              <button
                type="button"
                disabled={busy}
                className="btn btn-md btn-primary flex-1 disabled:cursor-not-allowed disabled:opacity-60"
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
              事件定义加载失败：{defsErr}
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
        <Panel title="保存事件分析报表">
          <div className="space-y-3 text-sm">
            <div>
              <div className="text-xs text-zinc-400">名称</div>
              <input
                value={saveName}
                onChange={(e) => setSaveName(e.target.value)}
                placeholder="例如 事件分析 - signup by plan"
                className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
            </div>
            <div>
              <div className="text-xs text-zinc-400">描述（可选）</div>
              <input
                value={saveDescription}
                onChange={(e) => setSaveDescription(e.target.value)}
                placeholder="例如 最近 7 天 signup 事件按 plan 拆分"
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
        <Panel title="事件分析报表列表">
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

      <EventAnalyticsResult
        result={result}
        eventDefs={eventDefs}
        chartType={chartType}
        onChartTypeChange={setChartType}
      />
    </div>
  );
}

function EventAnalyticsResult(props: { result: CustomAnalyticsResponse | null; eventDefs: EventDefinition[] | null; chartType: "line" | "column"; onChartTypeChange: (v: "line" | "column") => void }) {
  const { result, eventDefs, chartType, onChartTypeChange } = props;

  if (!result) {
    return (
      <Panel title="结果">
        <div className="text-sm text-zinc-500">尚未执行分析，可以先在上方配置并运行。</div>
      </Panel>
    );
  }
  const series = result.series ?? [];
  if (series.length === 0) {
    return (
      <Panel title="结果">
        <div className="text-sm text-zinc-500">暂无数据（当前时间/事件/属性条件下没有命中记录）。</div>
      </Panel>
    );
  }

  const title =
    result.group_by.includes("time") && result.group_by.length > 1
      ? "多维度时间序列"
      : result.group_by.includes("time")
        ? "时间序列"
        : "聚合结果";

  const hasTime = result.group_by.includes("time");

  return (
    <Panel
      title={title}
      right={
        <div className="flex items-center gap-4 text-xs text-zinc-500">
          <div>
            时间范围：{result.start.slice(0, 10)} ~ {result.end.slice(0, 10)}（粒度：
            {result.granularity}）
          </div>
          {hasTime ? (
            <div className="flex items-center gap-2">
              <span>图表类型</span>
              <div className="inline-flex overflow-hidden rounded border border-zinc-700 text-[11px]">
                <button
                  type="button"
                  className={
                    "px-2 py-0.5 " +
                    (chartType === "line"
                      ? "bg-zinc-800 text-zinc-50"
                      : "bg-zinc-900 text-zinc-400")
                  }
                  onClick={() => onChartTypeChange("line")}
                >
                  折线
                </button>
                <button
                  type="button"
                  className={
                    "px-2 py-0.5 border-l border-zinc-700 " +
                    (chartType === "column"
                      ? "bg-zinc-800 text-zinc-50"
                      : "bg-zinc-900 text-zinc-400")
                  }
                  onClick={() => onChartTypeChange("column")}
                >
                  柱状
                </button>
              </div>
            </div>
          ) : null}
        </div>
      }
    >
      <div className="space-y-4">
        {hasTime ? (
          <div className="space-y-3">
            {series.map((s: CustomAnalyticsSeries) => (
              <SeriesSparkline
                key={s.name}
                series={s}
                eventDefs={eventDefs}
                chartType={chartType}
              />
            ))}
          </div>
        ) : null}

        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-xs text-zinc-500">
              <tr>
                <th className="py-2 pr-4">Series</th>
                <th className="py-2 pr-4">Total</th>
                {hasTime ? <th className="py-2 pr-4">Last Point</th> : null}
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {series.map((s: CustomAnalyticsSeries) => {
                const last = s.points.at(-1);
                return (
                  <tr key={s.name} className="hover:bg-zinc-900/40">
                    <td className="py-2 pr-4 text-zinc-100">
                      <span className="block max-w-[28rem] truncate" title={getSeriesLabel(s, eventDefs)}>
                        {getSeriesLabel(s, eventDefs)}
                      </span>
                    </td>
                    <td className="py-2 pr-4 font-mono text-xs text-zinc-300">
                      {s.total}
                    </td>
                    {hasTime ? (
                      <td className="py-2 pr-4 font-mono text-xs text-zinc-400">
                        {last ? `${last.time}: ${last.value}` : "-"}
                      </td>
                    ) : null}
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

function getSeriesLabel(series: CustomAnalyticsSeries, eventDefs: EventDefinition[] | null): string {
  const eventName = series.dimensions?.event;
  const propValue = series.dimensions?.property;
  let labelParts: string[] = [];

  if (eventName) {
    const def = eventDefs?.find((d) => d.name === eventName);
    if (def) {
      const base = def.display_name && def.display_name !== def.name
        ? `${def.display_name} (${def.name})`
        : def.name;
      labelParts.push(base);
    } else {
      labelParts.push(eventName);
    }
  }

  if (propValue) {
    labelParts.push(String(propValue));
  }

  if (labelParts.length === 0) {
    return series.name;
  }
  return labelParts.join(" · ");
}

function SeriesSparkline(props: { series: CustomAnalyticsSeries; eventDefs: EventDefinition[] | null; chartType: "line" | "column" }) {
  const { series, eventDefs, chartType } = props;
  const values = series.points.map((p: CustomAnalyticsSeriesPoint) => p.value);
  const label = getSeriesLabel(series, eventDefs);

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
      <Sparkline values={values} variant={chartType === "column" ? "column" : "line"} />
    </div>
  );
}

function parseEventNames(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

function buildRangeFromDays(days: number): { start: string; end: string } {
  const safeDays = Number.isFinite(days) && days > 0 ? Math.min(days, 180) : 7;
  const end = new Date();
  const start = new Date(end.getTime() - (safeDays - 1) * 24 * 60 * 60 * 1000);
  return {
    start: start.toISOString(),
    end: end.toISOString(),
  };
}

function getRangeDays(startIso: string, endIso: string, fallbackDays: number): number {
  const start = new Date(startIso);
  const end = new Date(endIso);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
    return fallbackDays;
  }
  const diffMs = Math.max(0, end.getTime() - start.getTime());
  return Math.floor(diffMs / (24 * 60 * 60 * 1000)) + 1;
}
