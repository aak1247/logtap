export type JsonViewNode =
  | JsonPanelNode
  | JsonStackNode
  | JsonGridNode
  | JsonTextNode
  | JsonMetricNode
  | JsonStatusNode
  | JsonKVNode
  | JsonCodeNode
  | JsonLinkNode
  | JsonTableNode;

export type JsonViewNodeBase = {
  id?: string;
  className?: string;
};

export type JsonPanelNode = JsonViewNodeBase & {
  type: "panel";
  title?: string;
  children?: JsonViewNode[];
};

export type JsonStackNode = JsonViewNodeBase & {
  type: "stack";
  gap?: "xs" | "sm" | "md";
  children?: JsonViewNode[];
};

export type JsonGridNode = JsonViewNodeBase & {
  type: "grid";
  columns?: 1 | 2 | 3 | 4;
  children?: JsonViewNode[];
};

export type JsonTextNode = JsonViewNodeBase & {
  type: "text";
  text?: string;
  valuePath?: string;
  tone?: "default" | "muted" | "warning" | "danger" | "ok";
};

export type JsonMetricNode = JsonViewNodeBase & {
  type: "metric";
  label: string;
  value?: string | number | boolean | null;
  valuePath?: string;
  unit?: string;
  hint?: string;
  tone?: "default" | "muted" | "warning" | "danger" | "ok";
};

export type JsonStatusNode = JsonViewNodeBase & {
  type: "status";
  label?: string;
  value?: string | number | boolean | null;
  valuePath?: string;
  tone?: "default" | "muted" | "warning" | "danger" | "ok";
};

export type JsonKVNode = JsonViewNodeBase & {
  type: "kv";
  items: Array<{
    label: string;
    value?: string | number | boolean | null;
    valuePath?: string;
  }>;
};

export type JsonCodeNode = JsonViewNodeBase & {
  type: "code";
  value?: unknown;
  valuePath?: string;
};

export type JsonLinkNode = JsonViewNodeBase & {
  type: "link";
  label: string;
  href: string;
};

export type JsonTableNode = JsonViewNodeBase & {
  type: "table";
  title?: string;
  valuePath: string;
  columns: Array<{
    label: string;
    valuePath: string;
    unit?: string;
  }>;
};

export type JsonViewData = Record<string, unknown>;
