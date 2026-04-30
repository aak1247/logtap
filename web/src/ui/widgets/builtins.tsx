import { widgetRegistry } from "./registry";
import { UptimeOverview } from "../components/uptime/UptimeOverview";
import { ErrorTrendWidget } from "../components/ErrorTrendWidget";
import { SearchWidget } from "../components/SearchWidget";
import type { WidgetProps } from "./registry";

function PlaceholderWidget(props: WidgetProps & { label: string }) {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-950 p-4 text-sm text-zinc-500">
      {props.label} — 待实现
    </div>
  );
}

widgetRegistry.register({
  type: "uptime_overview",
  detectorType: "http_check",
  component: UptimeOverview,
  defaultSize: { w: 4, h: 3 },
  title: "可用性概览",
});

widgetRegistry.register({
  type: "tcp_status_card",
  detectorType: "tcp_check",
  component: (props: WidgetProps) => <PlaceholderWidget {...props} label="TCP 检查" />,
  defaultSize: { w: 2, h: 2 },
  title: "TCP 状态",
});

widgetRegistry.register({
  type: "dns_status_card",
  detectorType: "dns_check",
  component: (props: WidgetProps) => <PlaceholderWidget {...props} label="DNS 检查" />,
  defaultSize: { w: 2, h: 2 },
  title: "DNS 状态",
});

widgetRegistry.register({
  type: "ssl_status_card",
  detectorType: "ssl_check",
  component: (props: WidgetProps) => <PlaceholderWidget {...props} label="SSL 检查" />,
  defaultSize: { w: 2, h: 2 },
  title: "SSL 状态",
});

widgetRegistry.register({
  type: "error_trend",
  component: ErrorTrendWidget,
  defaultSize: { w: 4, h: 2 },
  title: "错误趋势",
});

widgetRegistry.register({
  type: "search_widget",
  component: SearchWidget,
  defaultSize: { w: 4, h: 1 },
  title: "搜索",
});
