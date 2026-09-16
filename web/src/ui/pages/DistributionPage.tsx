import { useEffect, useMemo, useState } from "react";
import {
  getDistributionSeries,
  type DistributionBucket,
  type DistributionDim,
  type DistributionMetric,
  type DistributionSeriesResponse,
} from "../../lib/api";
import { loadSettings } from "../../lib/storage";
import { useDebouncedValue } from "../../lib/useDebouncedValue";
import { Panel } from "../components/Panel";

const DIM_OPTIONS: { value: DistributionDim; label: string }[] = [
  { value: "country", label: "国家/地区" },
  { value: "region", label: "省份/州" },
  { value: "city", label: "城市" },
  { value: "asn_org", label: "运营商/组织" },
  { value: "os", label: "终端系统" },
  { value: "browser", label: "浏览器" },
];

const BUCKET_OPTIONS: { value: DistributionBucket; label: string }[] = [
  { value: "day", label: "按天" },
  { value: "week", label: "按周" },
  { value: "month", label: "按月" },
  { value: "year", label: "按年" },
];

const METRIC_OPTIONS: { value: DistributionMetric; label: string }[] = [
  { value: "users", label: "用户数" },
  { value: "events", label: "上报量" },
];

export function DistributionPage() {
  const settings = useMemo(() => loadSettings(), []);
  const [dim, setDim] = useState<DistributionDim>(() => readDimFromURL());
  const [bucket, setBucket] = useState<DistributionBucket>("day");
  const [metric, setMetric] = useState<DistributionMetric>("users");
  const [limit, setLimit] = useState(10);
  const debouncedLimit = useDebouncedValue(limit);
  const [range, setRange] = useState(() => defaultRange("day"));
  const [data, setData] = useState<DistributionSeriesResponse | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setLoading(true);
        setErr("");
        const res = await getDistributionSeries(settings, {
          dim,
          bucket,
          metric,
          limit: debouncedLimit,
          start: range.start,
          end: range.end,
        });
        if (!cancelled) setData(res);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [
    settings.apiBase,
    settings.projectId,
    settings.token,
    dim,
    bucket,
    metric,
    debouncedLimit,
    range.start,
    range.end,
  ]);

  const flattened = useMemo(() => flattenSeries(data), [data]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">分布分析</h1>
          <div className="mt-1 text-sm text-zinc-500">
            GeoIP City 数据可提供国家、省份/州、城市维度。
          </div>
        </div>
        <a href="/analytics" className="btn btn-sm btn-outline">
          返回分析
        </a>
      </div>

      {err ? (
        <div className="rounded-md border border-red-900/60 bg-red-950/30 px-3 py-2 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel title="筛选">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
          <SelectField
            label="维度"
            value={dim}
            options={DIM_OPTIONS}
            onChange={(next) => setDim(next as DistributionDim)}
          />
          <SelectField
            label="指标"
            value={metric}
            options={METRIC_OPTIONS}
            onChange={(next) => setMetric(next as DistributionMetric)}
          />
          <SelectField
            label="周期"
            value={bucket}
            options={BUCKET_OPTIONS}
            onChange={(next) => {
              const b = next as DistributionBucket;
              setBucket(b);
              setRange(defaultRange(b));
            }}
          />
          <div>
            <div className="text-xs text-zinc-400">Top N</div>
            <input
              type="number"
              min={1}
              max={100}
              value={limit}
              onChange={(e) => setLimit(clampLimit(e.target.value))}
              className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
            />
          </div>
          <div className="md:col-span-4">
            <PeriodRangePicker
              bucket={bucket}
              start={range.start}
              end={range.end}
              onChange={setRange}
            />
          </div>
        </div>
      </Panel>

      <Panel title={`${dimLabel(dim)} ${bucketLabel(bucket)}分布`}>
        {loading && !data ? (
          <div className="text-sm text-zinc-500">加载中...</div>
        ) : data && data.series.length > 0 ? (
          <DistributionSeriesTable data={data} />
        ) : (
          <div className="text-sm text-zinc-500">
            暂无分布数据。国家/省份/城市需要配置 GeoIP City
            mmdb，并在新数据上报后产生。
          </div>
        )}
      </Panel>

      <Panel title="汇总排行">
        {flattened.length > 0 ? (
          <DistBars items={flattened} />
        ) : (
          <div className="text-sm text-zinc-500">暂无数据</div>
        )}
      </Panel>
    </div>
  );
}

function PeriodRangePicker(props: {
  bucket: DistributionBucket;
  start: string;
  end: string;
  onChange: (range: { start: string; end: string }) => void;
}) {
  if (props.bucket === "week") {
    return (
      <PeriodInputs
        label="周范围"
        type="week"
        startValue={isoToWeekValue(props.start)}
        endValue={isoToWeekValue(props.end)}
        onStartChange={(value) =>
          props.onChange({ start: weekToStartIso(value), end: props.end })
        }
        onEndChange={(value) =>
          props.onChange({ start: props.start, end: weekToEndIso(value) })
        }
      />
    );
  }
  if (props.bucket === "month") {
    return (
      <PeriodInputs
        label="月份范围"
        type="month"
        startValue={isoToMonthValue(props.start)}
        endValue={isoToMonthValue(props.end)}
        onStartChange={(value) =>
          props.onChange({ start: monthToStartIso(value), end: props.end })
        }
        onEndChange={(value) =>
          props.onChange({ start: props.start, end: monthToEndIso(value) })
        }
      />
    );
  }
  if (props.bucket === "year") {
    return (
      <PeriodInputs
        label="年份范围"
        type="number"
        startValue={String(new Date(props.start).getUTCFullYear())}
        endValue={String(new Date(props.end).getUTCFullYear())}
        min="2000"
        max="2100"
        onStartChange={(value) =>
          props.onChange({ start: yearToStartIso(value), end: props.end })
        }
        onEndChange={(value) =>
          props.onChange({ start: props.start, end: yearToEndIso(value) })
        }
      />
    );
  }
  return (
    <PeriodInputs
      label="日期范围"
      type="date"
      startValue={isoToDateValue(props.start)}
      endValue={isoToDateValue(props.end)}
      onStartChange={(value) =>
        props.onChange({ start: dateToStartIso(value), end: props.end })
      }
      onEndChange={(value) =>
        props.onChange({ start: props.start, end: dateToEndIso(value) })
      }
    />
  );
}

function PeriodInputs(props: {
  label: string;
  type: "date" | "week" | "month" | "number";
  startValue: string;
  endValue: string;
  min?: string;
  max?: string;
  onStartChange: (value: string) => void;
  onEndChange: (value: string) => void;
}) {
  return (
    <div className="space-y-2 rounded-md border border-zinc-900/80 bg-zinc-950/30 p-2">
      <div className="text-xs text-zinc-400">{props.label}</div>
      <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
        <label>
          <div className="text-xs text-zinc-500">开始</div>
          <input
            type={props.type}
            value={props.startValue}
            min={props.min}
            max={props.max}
            onChange={(e) => props.onStartChange(e.target.value)}
            className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          />
        </label>
        <label>
          <div className="text-xs text-zinc-500">结束</div>
          <input
            type={props.type}
            value={props.endValue}
            min={props.min}
            max={props.max}
            onChange={(e) => props.onEndChange(e.target.value)}
            className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          />
        </label>
      </div>
    </div>
  );
}

function SelectField(props: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  return (
    <label>
      <div className="text-xs text-zinc-400">{props.label}</div>
      <select
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
      >
        {props.options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function DistributionSeriesTable(props: { data: DistributionSeriesResponse }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-xs text-zinc-500">
          <tr>
            <th className="w-32 px-2 py-2 font-medium">周期</th>
            <th className="px-2 py-2 font-medium">分布</th>
          </tr>
        </thead>
        <tbody>
          {props.data.series.map((bucket) => (
            <tr key={bucket.bucket} className="border-t border-zinc-900">
              <td className="px-2 py-3 font-mono text-xs text-zinc-400">
                {bucket.bucket}
              </td>
              <td className="px-2 py-3">
                {bucket.items.length > 0 ? (
                  <DistBars items={bucket.items} compact />
                ) : (
                  <span className="text-xs text-zinc-600">无数据</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DistBars(props: {
  items: { key: string; count: number }[];
  compact?: boolean;
}) {
  const max = Math.max(...props.items.map((item) => item.count), 1);
  return (
    <div className={props.compact ? "space-y-1.5" : "space-y-2"}>
      {props.items.map((item) => (
        <div key={item.key} className="flex items-center gap-3">
          <div
            className="w-36 shrink-0 truncate text-xs text-zinc-200 md:w-44"
            title={item.key}
          >
            {item.key}
          </div>
          <div className="h-2 flex-1 overflow-hidden rounded bg-zinc-900">
            <div
              className="h-2 rounded bg-cyan-500/70"
              style={{ width: `${Math.round((item.count / max) * 100)}%` }}
            />
          </div>
          <div className="w-14 text-right font-mono text-xs text-zinc-400">
            {item.count}
          </div>
        </div>
      ))}
    </div>
  );
}

function flattenSeries(data: DistributionSeriesResponse | null) {
  const acc = new Map<string, number>();
  for (const bucket of data?.series ?? []) {
    for (const item of bucket.items) {
      acc.set(item.key, (acc.get(item.key) ?? 0) + item.count);
    }
  }
  return Array.from(acc.entries())
    .map(([key, count]) => ({ key, count }))
    .sort((a, b) => b.count - a.count || a.key.localeCompare(b.key));
}

function defaultRange(bucket: DistributionBucket) {
  const end = new Date();
  const start = new Date(end);
  if (bucket === "year") start.setFullYear(end.getFullYear() - 4);
  else if (bucket === "month") start.setMonth(end.getMonth() - 11);
  else if (bucket === "week") start.setDate(end.getDate() - 7 * 11);
  else start.setDate(end.getDate() - 29);
  return { start: start.toISOString(), end: end.toISOString() };
}

function readDimFromURL(): DistributionDim {
  const dim = new URLSearchParams(window.location.search).get("dim");
  return DIM_OPTIONS.some((option) => option.value === dim)
    ? (dim as DistributionDim)
    : "country";
}

function clampLimit(value: string): number {
  const n = Number(value);
  if (!Number.isFinite(n)) return 10;
  return Math.min(100, Math.max(1, Math.trunc(n)));
}

function dimLabel(dim: DistributionDim): string {
  return DIM_OPTIONS.find((option) => option.value === dim)?.label ?? dim;
}

function bucketLabel(bucket: DistributionBucket): string {
  return BUCKET_OPTIONS.find((option) => option.value === bucket)?.label ?? "";
}

function isoToDateValue(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;
}

function isoToMonthValue(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}`;
}

function isoToWeekValue(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const date = new Date(
    Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()),
  );
  const day = date.getUTCDay() || 7;
  date.setUTCDate(date.getUTCDate() + 4 - day);
  const yearStart = new Date(Date.UTC(date.getUTCFullYear(), 0, 1));
  const week = Math.ceil(
    ((date.getTime() - yearStart.getTime()) / 86400000 + 1) / 7,
  );
  return `${date.getUTCFullYear()}-W${pad(week)}`;
}

function dateToStartIso(value: string): string {
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) return new Date().toISOString();
  return new Date(Date.UTC(year, month - 1, day, 0, 0, 0, 0)).toISOString();
}

function dateToEndIso(value: string): string {
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) return new Date().toISOString();
  return new Date(
    Date.UTC(year, month - 1, day, 23, 59, 59, 999),
  ).toISOString();
}

function monthToStartIso(value: string): string {
  const [year, month] = value.split("-").map(Number);
  if (!year || !month) return new Date().toISOString();
  return new Date(Date.UTC(year, month - 1, 1, 0, 0, 0, 0)).toISOString();
}

function monthToEndIso(value: string): string {
  const [year, month] = value.split("-").map(Number);
  if (!year || !month) return new Date().toISOString();
  return new Date(Date.UTC(year, month, 0, 23, 59, 59, 999)).toISOString();
}

function weekToStartIso(value: string): string {
  const [yearPart, weekPart] = value.split("-W");
  const year = Number(yearPart);
  const week = Number(weekPart);
  if (!year || !week) return new Date().toISOString();
  return isoWeekStart(year, week).toISOString();
}

function weekToEndIso(value: string): string {
  const start = new Date(weekToStartIso(value));
  start.setUTCDate(start.getUTCDate() + 6);
  start.setUTCHours(23, 59, 59, 999);
  return start.toISOString();
}

function yearToStartIso(value: string): string {
  const year = Number(value);
  if (!year) return new Date().toISOString();
  return new Date(Date.UTC(year, 0, 1, 0, 0, 0, 0)).toISOString();
}

function yearToEndIso(value: string): string {
  const year = Number(value);
  if (!year) return new Date().toISOString();
  return new Date(Date.UTC(year, 11, 31, 23, 59, 59, 999)).toISOString();
}

function isoWeekStart(year: number, week: number): Date {
  const jan4 = new Date(Date.UTC(year, 0, 4));
  const day = jan4.getUTCDay() || 7;
  const monday = new Date(jan4);
  monday.setUTCDate(jan4.getUTCDate() + 1 - day + (week - 1) * 7);
  monday.setUTCHours(0, 0, 0, 0);
  return monday;
}

function pad(value: number): string {
  return String(value).padStart(2, "0");
}
