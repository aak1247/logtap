import { useMemo } from "react";

type DateTimePickerProps = {
  label: string;
  value: string;
  onChange: (nextIso: string) => void;
  placeholder?: string;
};

export function DateTimePicker(props: DateTimePickerProps) {
  const localValue = useMemo(() => isoToLocalDateTime(props.value), [props.value]);

  return (
    <div>
      <div className="text-xs text-zinc-400">{props.label}</div>
      <input
        type="datetime-local"
        value={localValue}
        onChange={(e) => props.onChange(localDateTimeToIso(e.target.value))}
        placeholder={props.placeholder}
        className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
      />
      <div className="mt-1 truncate text-[11px] text-zinc-500">
        {props.value ? props.value : "未选择"}
      </div>
    </div>
  );
}

type TimeRangePickerProps = {
  label?: string;
  start: string;
  end: string;
  onStartChange: (nextIso: string) => void;
  onEndChange: (nextIso: string) => void;
  onRangePresetChange?: (startIso: string, endIso: string) => void;
};

const RANGE_PRESETS = [
  { label: "最近 1h", hours: 1 },
  { label: "最近 6h", hours: 6 },
  { label: "最近 24h", hours: 24 },
  { label: "最近 7d", hours: 24 * 7 },
  { label: "最近 30d", hours: 24 * 30 },
];

export function TimeRangePicker(props: TimeRangePickerProps) {
  const title = props.label ?? "时间范围";

  const applyPreset = (hours: number) => {
    const end = new Date();
    const start = new Date(end.getTime() - hours * 60 * 60 * 1000);
    const startIso = start.toISOString();
    const endIso = end.toISOString();
    props.onStartChange(startIso);
    props.onEndChange(endIso);
    props.onRangePresetChange?.(startIso, endIso);
  };

  return (
    <div className="space-y-2 rounded-md border border-zinc-900/80 bg-zinc-950/30 p-2">
      <div className="text-xs text-zinc-400">{title}</div>
      <div className="flex flex-wrap gap-1.5">
        {RANGE_PRESETS.map((preset) => (
          <button
            key={preset.label}
            type="button"
            className="rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1 text-xs text-zinc-200 transition hover:bg-zinc-800"
            onClick={() => applyPreset(preset.hours)}
          >
            {preset.label}
          </button>
        ))}
      </div>
      <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
        <DateTimePicker label="开始" value={props.start} onChange={props.onStartChange} />
        <DateTimePicker label="结束" value={props.end} onChange={props.onEndChange} />
      </div>
    </div>
  );
}

function isoToLocalDateTime(iso: string): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hour = pad(date.getHours());
  const minute = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hour}:${minute}`;
}

function localDateTimeToIso(local: string): string {
  if (!local) return "";
  const date = new Date(local);
  if (Number.isNaN(date.getTime())) return "";
  return date.toISOString();
}

function pad(v: number): string {
  return String(v).padStart(2, "0");
}
