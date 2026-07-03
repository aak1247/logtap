import { Link } from "react-router-dom";
import { Panel } from "../components/Panel";
import type { JsonViewData, JsonViewNode } from "./types";

type Props = {
  view: JsonViewNode;
  data?: JsonViewData;
};

export function JsonViewRenderer(props: Props) {
  return <JsonNode node={props.view} data={props.data ?? {}} />;
}

function JsonNode(props: { node: JsonViewNode; data: JsonViewData }) {
  const { node, data } = props;
  switch (node.type) {
    case "panel":
      return (
        <Panel title={node.title ?? ""}>
          <div className="space-y-3">
            {(node.children ?? []).map((child, i) => (
              <JsonNode key={child.id ?? i} node={child} data={data} />
            ))}
          </div>
        </Panel>
      );
    case "stack":
      return (
        <div className={stackClass(node.gap)}>
          {(node.children ?? []).map((child, i) => (
            <JsonNode key={child.id ?? i} node={child} data={data} />
          ))}
        </div>
      );
    case "grid":
      return (
        <div className={gridClass(node.columns)}>
          {(node.children ?? []).map((child, i) => (
            <JsonNode key={child.id ?? i} node={child} data={data} />
          ))}
        </div>
      );
    case "text": {
      const value = node.valuePath ? readPath(data, node.valuePath) : node.text;
      return <div className={`text-sm ${toneText(node.tone)}`}>{formatValue(value)}</div>;
    }
    case "metric": {
      const value = node.valuePath ? readPath(data, node.valuePath) : node.value;
      return (
        <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3">
          <div className="text-xs text-zinc-500">{node.label}</div>
          <div className={`mt-1 text-2xl font-semibold ${toneText(node.tone)}`}>
            {formatValue(value)}
            {node.unit ? <span className="ml-1 text-sm text-zinc-500">{node.unit}</span> : null}
          </div>
          {node.hint ? <div className="mt-1 text-xs text-zinc-500">{node.hint}</div> : null}
        </div>
      );
    }
    case "status": {
      const value = node.valuePath ? readPath(data, node.valuePath) : node.value;
      return (
        <div className="flex items-center justify-between gap-3 text-sm">
          {node.label ? <span className="text-zinc-500">{node.label}</span> : null}
          <span className={`rounded-md px-2 py-1 text-xs ${toneBadge(node.tone)}`}>
            {formatValue(value)}
          </span>
        </div>
      );
    }
    case "kv":
      return (
        <dl className="divide-y divide-zinc-900 text-sm">
          {node.items.map((item) => {
            const value = item.valuePath ? readPath(data, item.valuePath) : item.value;
            return (
              <div key={item.label} className="flex justify-between gap-4 py-2">
                <dt className="text-zinc-500">{item.label}</dt>
                <dd className="truncate text-right text-zinc-200">{formatValue(value)}</dd>
              </div>
            );
          })}
        </dl>
      );
    case "code": {
      const value = node.valuePath ? readPath(data, node.valuePath) : node.value;
      return (
        <pre className="max-h-72 overflow-auto rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-xs text-zinc-300">
          {JSON.stringify(value ?? {}, null, 2)}
        </pre>
      );
    }
    case "link":
      return (
        <Link className="text-sm text-indigo-400 hover:text-indigo-300" to={node.href}>
          {node.label}
        </Link>
      );
    case "table": {
      const rowsRaw = readPath(data, node.valuePath);
      const rows = Array.isArray(rowsRaw) ? rowsRaw : [];
      return (
        <div className="overflow-x-auto rounded-lg border border-zinc-800">
          {node.title ? (
            <div className="border-b border-zinc-800 bg-zinc-950 px-3 py-2 text-sm font-medium text-zinc-200">
              {node.title}
            </div>
          ) : null}
          <table className="w-full text-left text-sm">
            <thead className="bg-zinc-950 text-xs text-zinc-500">
              <tr>
                {node.columns.map((col) => (
                  <th key={col.label} className="px-3 py-2 font-medium">
                    {col.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {rows.map((row, i) => (
                <tr key={i} className="hover:bg-zinc-900/40">
                  {node.columns.map((col) => {
                    const value =
                      row && typeof row === "object"
                        ? readPath(row as JsonViewData, col.valuePath)
                        : undefined;
                    return (
                      <td key={col.label} className="px-3 py-2 text-zinc-300">
                        {formatValueWithUnit(value, col.unit)}
                      </td>
                    );
                  })}
                </tr>
              ))}
              {rows.length === 0 ? (
                <tr>
                  <td className="px-3 py-4 text-zinc-500" colSpan={node.columns.length}>
                    暂无数据
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      );
    }
  }
}

function readPath(data: JsonViewData, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in acc) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, data);
}

function formatValue(value: unknown): string {
  if (value === undefined || value === null || value === "") return "-";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function formatValueWithUnit(value: unknown, unit: string | undefined): string {
  const formatted = formatValue(value);
  if (!unit || formatted === "-") return formatted;
  return `${formatted} ${unit}`;
}

function stackClass(gap: "xs" | "sm" | "md" | undefined): string {
  if (gap === "xs") return "space-y-1";
  if (gap === "md") return "space-y-4";
  return "space-y-2";
}

function gridClass(columns: 1 | 2 | 3 | 4 | undefined): string {
  const base = "grid grid-cols-1 gap-3";
  if (columns === 4) return `${base} md:grid-cols-4`;
  if (columns === 3) return `${base} md:grid-cols-3`;
  if (columns === 2) return `${base} md:grid-cols-2`;
  return base;
}

function toneText(tone: string | undefined): string {
  switch (tone) {
    case "ok":
      return "text-emerald-300";
    case "warning":
      return "text-amber-300";
    case "danger":
      return "text-red-300";
    case "muted":
      return "text-zinc-500";
    default:
      return "text-zinc-200";
  }
}

function toneBadge(tone: string | undefined): string {
  switch (tone) {
    case "ok":
      return "border border-emerald-800 bg-emerald-950 text-emerald-200";
    case "warning":
      return "border border-amber-800 bg-amber-950 text-amber-200";
    case "danger":
      return "border border-red-800 bg-red-950 text-red-200";
    case "muted":
      return "border border-zinc-800 bg-zinc-900 text-zinc-400";
    default:
      return "border border-zinc-800 bg-zinc-900 text-zinc-200";
  }
}
