import { useEffect, useState } from "react";
import { searchLogs, type LogRow } from "../../lib/api";
import { Sparkline } from "./Sparkline";
import type { WidgetProps } from "../widgets/registry";

export function ErrorTrendWidget(props: WidgetProps) {
  const { settings } = props;
  const [points, setPoints] = useState<number[]>([]);
  const [topErrors, setTopErrors] = useState<{ msg: string; count: number }[]>([]);

  useEffect(() => {
    if (!settings.token || !settings.projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const end = new Date();
        const buckets: number[] = [];
        const errors: Map<string, number> = new Map();

        for (let i = 23; i >= 0; i--) {
          const bucketEnd = new Date(end.getTime() - i * 3600000);
          const bucketStart = new Date(bucketEnd.getTime() - 3600000);
          try {
            const rows: LogRow[] = await searchLogs(settings, {
              level: "error",
              start: bucketStart.toISOString(),
              end: bucketEnd.toISOString(),
              limit: 1000,
            });
            if (cancelled) return;
            buckets.push(rows.length);
            for (const r of rows) {
              const key = r.message?.slice(0, 80) || "(empty)";
              errors.set(key, (errors.get(key) || 0) + 1);
            }
          } catch {
            buckets.push(0);
          }
        }

        if (!cancelled) {
          setPoints(buckets);
          setTopErrors(
            Array.from(errors.entries())
              .sort((a, b) => b[1] - a[1])
              .slice(0, 10)
              .map(([msg, count]) => ({ msg, count })),
          );
        }
      } catch {}
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.token, settings.projectId]);

  return (
    <div className="space-y-4">
      <div>
        <div className="text-sm font-medium text-zinc-300">错误趋势 (24h)</div>
        {points.length > 0 && <Sparkline values={points} width={400} height={60} />}
      </div>
      {topErrors.length > 0 && (
        <div>
          <div className="mb-2 text-sm font-medium text-zinc-300">Top 错误</div>
          <div className="space-y-1">
            {topErrors.map((e, i) => (
              <div
                key={i}
                className="flex items-center justify-between rounded bg-zinc-900/50 px-3 py-1.5 text-xs"
              >
                <span className="truncate text-zinc-300" title={e.msg}>
                  {e.msg}
                </span>
                <span className="ml-2 font-mono text-red-400">{e.count}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
