import { useEffect, useState, useMemo } from "react";
import {
  listMonitors,
  listMonitorRuns,
  getDetectorAggregate,
  type MonitorDefinition,
  type MonitorRun,
  type AggregatePoint,
} from "../../../lib/api";
import { Sparkline } from "../Sparkline";
import type { WidgetProps } from "../../widgets/registry";

type StatusSegment = { ts: string; ok: boolean };

function UptimeBar(props: { segments: StatusSegment[] }) {
  if (!props.segments.length) return null;
  const width = 100;
  const segW = Math.max(width / props.segments.length, 2);
  return (
    <svg width={width} height={14} className="inline-block">
      {props.segments.map((s, i) => (
        <rect
          key={i}
          x={i * segW}
          y={0}
          width={segW - 0.5}
          height={14}
          rx={1}
          className={s.ok ? "fill-emerald-500" : "fill-red-500"}
        />
      ))}
    </svg>
  );
}

function UptimeCard(props: {
  monitor: MonitorDefinition;
  runs: MonitorRun[];
  points: AggregatePoint[];
}) {
  const { monitor, runs, points } = props;
  const [expanded, setExpanded] = useState(false);

  const totalRuns = runs.length || 1;
  const successRuns = runs.filter((r) => r.status === "success").length;
  const uptimePct = ((successRuns / totalRuns) * 100).toFixed(1);

  const segments: StatusSegment[] = useMemo(
    () => runs.map((r) => ({ ts: r.started_at, ok: r.status === "success" })),
    [runs],
  );

  const values = useMemo(() => points.map((p) => p.value), [points]);

  return (
    <div
      className="cursor-pointer rounded-xl border border-zinc-800 bg-zinc-950 p-4 transition-colors hover:border-zinc-700"
      onClick={() => setExpanded((e) => !e)}
    >
      <div className="flex items-center justify-between">
        <div>
          <div className="text-sm font-medium text-zinc-200">{monitor.name}</div>
          <div className="mt-0.5 text-xs text-zinc-500">{monitor.detector_type}</div>
        </div>
        <div className="text-right">
          <div className="text-xl font-bold text-emerald-400">{uptimePct}%</div>
          <div className="text-[10px] text-zinc-500">可用率</div>
        </div>
      </div>
      <div className="mt-3 flex items-center gap-3">
        <UptimeBar segments={segments} />
        <span className="text-[10px] text-zinc-500">24h</span>
      </div>
      {values.length > 1 && (
        <div className="mt-2">
          <Sparkline values={values} width={200} height={40} />
        </div>
      )}
      {expanded && (
        <div className="mt-3 border-t border-zinc-800 pt-3">
          <div className="text-xs text-zinc-400">
            <div>间隔: {monitor.interval_sec}s</div>
            <div>超时: {monitor.timeout_ms}ms</div>
            <div>最近执行: {runs.length} 次</div>
            <div>
              配置:{" "}
              <pre className="mt-1 max-h-32 overflow-auto rounded bg-zinc-900 p-2 text-[10px]">
                {JSON.stringify(monitor.config, null, 2)}
              </pre>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export function UptimeOverview(props: WidgetProps) {
  const { settings } = props;
  const [monitors, setMonitors] = useState<MonitorDefinition[]>([]);
  const [runsMap, setRunsMap] = useState<Map<number, MonitorRun[]>>(new Map());
  const [aggMap, setAggMap] = useState<Map<number, AggregatePoint[]>>(new Map());

  useEffect(() => {
    if (!settings.token || !settings.projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const { items } = await listMonitors(settings);
        if (cancelled) return;
        const httpMonitors = items.filter(
          (m) =>
            m.detector_type === "http_check" ||
            m.detector_type === "tcp_check" ||
            m.detector_type === "dns_check" ||
            m.detector_type === "ssl_check",
        );
        setMonitors(httpMonitors);

        const newRuns = new Map<number, MonitorRun[]>();
        const newAgg = new Map<number, AggregatePoint[]>();

        await Promise.all(
          httpMonitors.map(async (m) => {
            try {
              const r = await listMonitorRuns(settings, m.id, { limit: 48 });
              if (!cancelled) newRuns.set(m.id, r.items);
            } catch {}
            try {
              const end = new Date().toISOString();
              const start = new Date(Date.now() - 86400000).toISOString();
              const a = await getDetectorAggregate(settings, m.detector_type, {
                start,
                end,
                interval: "1h",
              });
              if (!cancelled) newAgg.set(m.id, a.points || []);
            } catch {}
          }),
        );

        if (!cancelled) {
          setRunsMap(newRuns);
          setAggMap(newAgg);
        }
      } catch {}
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.token, settings.projectId]);

  if (!monitors.length) {
    return (
      <div className="rounded-xl border border-zinc-800 bg-zinc-950 p-4 text-sm text-zinc-500">
        暂无可用性监控，请在"告警 → 监控"中添加 HTTP/TCP/DNS/SSL 检查。
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
      {monitors.map((m) => (
        <UptimeCard
          key={m.id}
          monitor={m}
          runs={runsMap.get(m.id) || []}
          points={aggMap.get(m.id) || []}
        />
      ))}
    </div>
  );
}
