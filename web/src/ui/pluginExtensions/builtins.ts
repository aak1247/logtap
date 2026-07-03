import { pluginExtensionRegistry } from "./registry";

const packageId = "builtin.connectivity";

pluginExtensionRegistry.register({
  id: "builtin.connectivity.tcp_check.overview",
  packageId,
  detectorType: "tcp_check",
  surface: "overview_card",
  title: "TCP 状态",
  view: {
    type: "stack",
    gap: "md",
    children: [
      {
        type: "grid",
        columns: 3,
        children: [
          { type: "metric", label: "状态", valuePath: "summary.status" },
          { type: "metric", label: "成功率", valuePath: "summary.successRate", unit: "%" },
          { type: "metric", label: "平均延迟", valuePath: "summary.avgElapsedMs", unit: "ms" },
        ],
      },
      {
        type: "table",
        valuePath: "monitors",
        columns: [
          { label: "名称", valuePath: "name" },
          { label: "目标", valuePath: "target" },
          { label: "状态", valuePath: "status" },
          { label: "最近错误", valuePath: "lastError" },
        ],
      },
    ],
  },
});

pluginExtensionRegistry.register({
  id: "builtin.connectivity.dns_check.overview",
  packageId,
  detectorType: "dns_check",
  surface: "overview_card",
  title: "DNS 状态",
  view: {
    type: "stack",
    gap: "md",
    children: [
      {
        type: "grid",
        columns: 3,
        children: [
          { type: "metric", label: "状态", valuePath: "summary.status" },
          { type: "metric", label: "成功率", valuePath: "summary.successRate", unit: "%" },
          { type: "metric", label: "平均延迟", valuePath: "summary.avgElapsedMs", unit: "ms" },
        ],
      },
      {
        type: "table",
        valuePath: "monitors",
        columns: [
          { label: "名称", valuePath: "name" },
          { label: "目标", valuePath: "target" },
          { label: "状态", valuePath: "status" },
          { label: "最近错误", valuePath: "lastError" },
        ],
      },
    ],
  },
});

pluginExtensionRegistry.register({
  id: "builtin.connectivity.ssl_check.overview",
  packageId,
  detectorType: "ssl_check",
  surface: "overview_card",
  title: "SSL 状态",
  view: {
    type: "stack",
    gap: "md",
    children: [
      {
        type: "grid",
        columns: 3,
        children: [
          { type: "metric", label: "状态", valuePath: "summary.status" },
          { type: "metric", label: "最短剩余", valuePath: "summary.worstCertDays", unit: "天" },
          { type: "metric", label: "平均延迟", valuePath: "summary.avgElapsedMs", unit: "ms" },
        ],
      },
      {
        type: "table",
        valuePath: "monitors",
        columns: [
          { label: "名称", valuePath: "name" },
          { label: "目标", valuePath: "target" },
          { label: "状态", valuePath: "status" },
          { label: "最近错误", valuePath: "lastError" },
        ],
      },
    ],
  },
});

pluginExtensionRegistry.register({
  id: "builtin.connectivity.analysis",
  packageId,
  surface: "analytics_tab",
  title: "连接性分析",
  view: {
    type: "panel",
    title: "连接性分析",
    children: [
      {
        type: "grid",
        columns: 3,
        children: [
          { type: "metric", label: "TCP 检查", value: "待接入", tone: "muted" },
          { type: "metric", label: "DNS 检查", value: "待接入", tone: "muted" },
          { type: "metric", label: "SSL 检查", value: "待接入", tone: "muted" },
        ],
      },
      { type: "text", text: "这里预留插件自定义分析 tab，后续接入 analysis API 后展示真实聚合结果。", tone: "muted" },
    ],
  },
});
