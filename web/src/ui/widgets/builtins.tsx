import { widgetRegistry } from "./registry";
import { UptimeOverview } from "../components/uptime/UptimeOverview";
import { ErrorTrendWidget } from "../components/ErrorTrendWidget";

widgetRegistry.register({
  type: "uptime_overview",
  detectorType: "http_check",
  component: UptimeOverview,
  defaultSize: { w: 4, h: 3 },
  title: "可用性概览",
});

widgetRegistry.register({
  type: "error_trend",
  component: ErrorTrendWidget,
  defaultSize: { w: 4, h: 2 },
  title: "错误趋势",
});
