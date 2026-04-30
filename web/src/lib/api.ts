import { clearAuth } from "./storage";

export type ApiSettings = {
  apiBase: string;
  token: string;
  projectId: string;
};

type ApiEnvelope<T> = {
  code: number;
  err?: string;
  data?: T;
};

export type MetricsToday = {
  project_id: number;
  date: string;
  logs: number;
  events: number;
  errors: number;
  users: number;
};

export type MetricsTotal = {
  project_id: number;
  logs: number;
  events: number;
  users: number;
};

export type StorageEstimateTable = {
  count: number;
  sample_size: number;
  avg_row_bytes: number;
  est_bytes: number;
};

export type StorageEstimate = {
  project_id: number;
  logs: StorageEstimateTable;
  events: StorageEstimateTable;
  total_bytes: number;
  estimated_at: string;
};

export type CleanupPolicy = {
  project_id: number;
  enabled: boolean;
  logs_retention_days: number;
  events_retention_days: number;
  track_events_retention_days: number;
  schedule_hour_utc: number;
  schedule_minute_utc: number;
  last_run_at?: string;
  next_run_at?: string;
  created_at: string;
  updated_at: string;
};

export type User = { id: number; email: string };
export type SelfLogConfig = { project_id: string | number; project_key: string };
export type LoginResponse = { token: string; user: User; self_log?: SelfLogConfig };
export type BootstrapResponse = {
  token: string;
  user: User;
  project: { id: string; name: string };
  key: { id: number; name: string; key: string };
  self_log?: SelfLogConfig;
};

export type SystemStatusResponse = {
  status: "uninitialized" | "running" | "maintenance" | "exception";
  initialized: boolean;
  auth_enabled: boolean;
  message?: string;
};

export type Project = { id: string; owner_user_id: number; name: string };
export type ProjectKey = {
  id: number;
  project_id: string;
  name: string;
  key: string;
  created_at: string;
  revoked_at?: string;
};

export type AlertContact = {
  id: number;
  project_id: number;
  type: "email" | "sms";
  name: string;
  value: string;
  created_at: string;
  updated_at: string;
};

export type AlertContactGroup = {
  id: number;
  project_id: number;
  type: "email" | "sms";
  name: string;
  created_at: string;
  updated_at: string;
};

export type AlertContactGroupWithMembers = AlertContactGroup & {
  memberContactIds: number[];
};

export type AlertContactGroupUpsertResponse = {
  group: AlertContactGroup;
  memberContactIds: number[];
};

export type AlertWecomBot = {
  id: number;
  project_id: number;
  name: string;
  webhook_url: string;
  created_at: string;
  updated_at: string;
};

export type AlertWebhookEndpoint = {
  id: number;
  project_id: number;
  name: string;
  url: string;
  created_at: string;
  updated_at: string;
};

export type AlertRuleSource = "logs" | "events" | "both";

export type AlertRule = {
  id: number;
  project_id: number;
  name: string;
  enabled: boolean;
  source: AlertRuleSource;
  match: Record<string, unknown>;
  repeat: Record<string, unknown>;
  targets: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type AlertDelivery = {
  id: number;
  project_id: number;
  rule_id: number;
  channel_type: "wecom" | "webhook" | "email" | "sms";
  target: string;
  title: string;
  content: string;
  status: "pending" | "processing" | "sent" | "failed";
  attempts: number;
  next_attempt_at: string;
  last_error: string;
  created_at: string;
  updated_at: string;
};

export type AlertDeliveryPreview = {
  channelType: string;
  target: string;
  title: string;
  content: string;
};

export type EventDefinition = {
  id: number;
  project_id: number;
  name: string;
  display_name: string;
  category: string;
  description: string;
  status: string;
  owner: string;
  created_at: string;
  updated_at: string;
};

export type PropertyDefinition = {
  id: number;
  project_id: number;
  key: string;
  display_name: string;
  type: "string" | "enum" | "number" | string;
  description: string;
  status: string;
  enum_values?: string[] | null;
  example_values?: string[] | null;
  created_at: string;
  updated_at: string;
};

export type AlertRulePreview = {
  ruleId: number;
  ruleName: string;
  matched: boolean;
  dedupeKeyHash?: string;
  windowSec?: number;
  threshold?: number;
  occurrences?: number;
  occurrencesBefore?: number;
  occurrencesAfter?: number;
  backoffExpBefore?: number;
  backoffExpAfter?: number;
  nextAllowedAtBefore?: string;
  nextAllowedAtAfter?: string;
  windowExpired?: boolean;
  willEnqueue?: boolean;
  suppressedReason?: string;
  suppressedMessage?: string;
  deliveries?: AlertDeliveryPreview[];
};

export type DetectorDescriptor = {
  type: string;
  mode: string;
  path: string;
};

export type DetectorSchemaResponse = {
  detectorType: string;
  schema: unknown;
};

export type MonitorDefinition = {
  id: number;
  project_id: number;
  name: string;
  detector_type: string;
  config: Record<string, unknown>;
  interval_sec: number;
  timeout_ms: number;
  enabled: boolean;
  next_run_at: string;
  lease_owner: string;
  lease_until?: string;
  created_at: string;
  updated_at: string;
};

export type MonitorRun = {
  id: number;
  monitor_id: number;
  project_id: number;
  started_at: string;
  finished_at: string;
  status: string;
  signal_count: number;
  error: string;
  result: Record<string, unknown>;
  created_at: string;
};

export type MonitorUpsertRequest = {
  name: string;
  detectorType: string;
  config: Record<string, unknown>;
  intervalSec?: number;
  timeoutMs?: number;
  enabled?: boolean;
};

export type MonitorTestSample = {
  source: string;
  sourceType: string;
  severity: string;
  status: string;
  message: string;
  labels?: Record<string, string>;
  occurredAt: string;
};

export type ChannelDescriptor = {
  type: string;
  schema: unknown;
};

export type PluginHealthResponse = {
  status: string;
  message?: string;
  last_check?: string;
};

export type AggregatePoint = {
  time: string;
  value: number;
};

export type AggregateResponse = {
  detector_type: string;
  interval: string;
  points: AggregatePoint[];
};

export type SearchResult = {
  items: LogRow[];
  total: number;
  facets?: Record<string, { key: string; count: number }[]>;
};

export async function listChannels(
  s: ApiSettings,
): Promise<{ items: ChannelDescriptor[] }> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/channels`, s.token);
}

export async function getDetectorHealth(
  s: ApiSettings,
  detectorType: string,
): Promise<PluginHealthResponse> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/plugins/detectors/${encodeURIComponent(detectorType)}/health`,
    s.token,
  );
}

export async function getDetectorAggregate(
  s: ApiSettings,
  detectorType: string,
  params: { projectId?: string; start?: string; end?: string; interval?: string },
): Promise<AggregateResponse> {
  const usp = new URLSearchParams();
  if (params.projectId) usp.set("project_id", params.projectId);
  if (params.start) usp.set("start", params.start);
  if (params.end) usp.set("end", params.end);
  if (params.interval) usp.set("interval", params.interval);
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/plugins/detectors/${encodeURIComponent(detectorType)}/aggregate?${usp.toString()}`,
    s.token,
  );
}

export async function searchUnified(
  s: ApiSettings,
  params: {
    q?: string;
    start?: string;
    end?: string;
    page?: number;
    pageSize?: number;
  },
): Promise<SearchResult> {
  const usp = new URLSearchParams();
  if (params.q) usp.set("q", params.q);
  if (params.start) usp.set("start", params.start);
  if (params.end) usp.set("end", params.end);
  if (params.page) usp.set("page", String(params.page));
  if (params.pageSize) usp.set("pageSize", String(params.pageSize));
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/search?${usp.toString()}`,
    s.token,
  );
}

export type MonitorTestResult = {
  monitorId: number;
  detectorType: string;
  signalCount: number;
  elapsedMs: number;
  samples: MonitorTestSample[];
};

export type RecentEvent = {
  id: string;
  timestamp: string;
  level?: string;
  title?: string;
};

export type LogRow = {
  id: number;
  timestamp: string;
  level?: string;
  trace_id?: string;
  span_id?: string;
  message: string;
  fields?: Record<string, unknown>;
};

export type BucketCount = { bucket: string; active: number };
export type ActiveSeriesResponse = {
  project_id: number;
  bucket: "day" | "month";
  start: string;
  end: string;
  series: BucketCount[];
};

export type DistItem = { key: string; count: number };
export type DistributionResponse = {
  project_id: number;
  dim: string;
  start: string;
  end: string;
  items: DistItem[];
};

export type RetentionPoint = { day: number; active: number; rate: number };
export type RetentionRow = {
  cohort: string;
  cohort_size: number;
  points: RetentionPoint[];
};
export type RetentionResponse = {
  project_id: number;
  start: string;
  end: string;
  days: number[];
  rows: RetentionRow[];
};

export type TopEventRow = { name: string; events: number; users: number };
export type TopEventsResponse = {
  project_id: number;
  start: string;
  end: string;
  items: TopEventRow[];
};

export type FunnelStep = {
  name: string;
  users: number;
  conversion: number;
  dropoff: number;
};
export type FunnelResponse = {
  project_id: number;
  start: string;
  end: string;
  within_secs: number;
  steps: FunnelStep[];
};

export type UserGrowthPoint = { day: string; new_users: number };
export type UserGrowthResponse = {
  project_id: number;
  start: string;
  end: string;
  series: UserGrowthPoint[];
  total_users: number;
};

export type CustomAnalyticsSeriesPoint = { time: string; value: number };
export type CustomAnalyticsSeries = {
  name: string;
  dimensions: Record<string, string>;
  points: CustomAnalyticsSeriesPoint[];
  total: number;
};
export type CustomAnalyticsResponse = {
  project_id: number;
  analysis_type: string;
  metric: string;
  granularity: string;
  start: string;
  end: string;
  group_by: string[];
  property_key?: string;
  series: CustomAnalyticsSeries[];
};

export type AnalysisView = {
  id: number;
  project_id: number;
  name: string;
  description: string;
  analysis_type: string;
  query: unknown;
  owner_user_id: number;
  created_at: string;
  updated_at: string;
};

export async function listAnalysisViews(
  s: ApiSettings,
  params?: { analysis_type?: string },
): Promise<{ items: AnalysisView[] }> {
  const usp = new URLSearchParams();
  if (params?.analysis_type) usp.set("analysis_type", params.analysis_type);
  const qs = usp.toString();
  const path = `${s.apiBase}/api/${s.projectId}/analytics/views${qs ? `?${qs}` : ""}`;
  return fetchJSON(path, s.token);
}

export async function createAnalysisView(
  s: ApiSettings,
  req: { name: string; description?: string; analysis_type: string; query: unknown },
): Promise<AnalysisView> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/analytics/views`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function getAnalysisView(
  s: ApiSettings,
  viewId: number,
): Promise<AnalysisView> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/views/${viewId}`,
    s.token,
  );
}

export async function deleteAnalysisView(
  s: ApiSettings,
  viewId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/views/${viewId}`,
    s.token,
    { method: "DELETE" },
  );
}

export async function listEventDefinitions(
  s: ApiSettings,
  params?: { status?: string; q?: string },
): Promise<{ items: EventDefinition[] }> {
  const usp = new URLSearchParams();
  if (params?.status) usp.set("status", params.status);
  if (params?.q) usp.set("q", params.q);
  const qs = usp.toString();
  const path = `${s.apiBase}/api/${s.projectId}/events/schema${qs ? `?${qs}` : ""}`;
  return fetchJSON(path, s.token);
}

export async function createEventDefinition(
  s: ApiSettings,
  req: {
    name: string;
    display_name?: string;
    category?: string;
    description?: string;
    status?: string;
    owner?: string;
  },
): Promise<EventDefinition> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/events/schema`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateEventDefinition(
  s: ApiSettings,
  name: string,
  req: {
    display_name?: string;
    category?: string;
    description?: string;
    status?: string;
    owner?: string;
  },
): Promise<EventDefinition> {
  const safeName = encodeURIComponent(name);
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/events/schema/${safeName}`,
    s.token,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export async function listPropertyDefinitions(
  s: ApiSettings,
  params?: { status?: string; type?: string; q?: string },
): Promise<{ items: PropertyDefinition[] }> {
  const usp = new URLSearchParams();
  if (params?.status) usp.set("status", params.status);
  if (params?.type) usp.set("type", params.type);
  if (params?.q) usp.set("q", params.q);
  const qs = usp.toString();
  const path = `${s.apiBase}/api/${s.projectId}/properties/schema${qs ? `?${qs}` : ""}`;
  return fetchJSON(path, s.token);
}

export async function createPropertyDefinition(
  s: ApiSettings,
  req: {
    key: string;
    display_name?: string;
    type?: string;
    description?: string;
    status?: string;
    enum_values?: string[];
    example_values?: string[];
  },
): Promise<PropertyDefinition> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/properties/schema`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updatePropertyDefinition(
  s: ApiSettings,
  key: string,
  req: {
    display_name?: string;
    type?: string;
    description?: string;
    status?: string;
    enum_values?: string[];
    example_values?: string[];
  },
): Promise<PropertyDefinition> {
  const safeKey = encodeURIComponent(key);
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/properties/schema/${safeKey}`,
    s.token,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    },
  );
}

export async function getMetricsToday(s: ApiSettings): Promise<MetricsToday> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/metrics/today`, s.token);
}

export async function getMetricsTotal(s: ApiSettings): Promise<MetricsTotal> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/metrics/total`, s.token);
}

export async function getUserGrowth(
  s: ApiSettings,
  params?: { start?: string; end?: string },
): Promise<UserGrowthResponse> {
  const usp = new URLSearchParams();
  if (params?.start) usp.set("start", params.start);
  if (params?.end) usp.set("end", params.end);
  const qs = usp.toString();
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/users${qs ? `?${qs}` : ""}`,
    s.token,
  );
}

export async function getStorageEstimate(s: ApiSettings): Promise<StorageEstimate> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/storage/estimate`, s.token);
}

export async function getCleanupPolicy(s: ApiSettings): Promise<CleanupPolicy> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/cleanup/policy`, s.token);
}

export async function upsertCleanupPolicy(
  s: ApiSettings,
  req: Partial<
    Pick<
      CleanupPolicy,
      | "enabled"
      | "logs_retention_days"
      | "events_retention_days"
      | "track_events_retention_days"
      | "schedule_hour_utc"
      | "schedule_minute_utc"
    >
  >,
): Promise<CleanupPolicy> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/cleanup/policy`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function runCleanupPolicy(
  s: ApiSettings,
): Promise<{
  project_id: number;
  logs_deleted: number;
  events_deleted: number;
  track_events_deleted: number;
  logs_before?: string;
  events_before?: string;
  track_events_before?: string;
  ran_at: string;
}> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/cleanup/run`, s.token, {
    method: "POST",
  });
}

export async function cleanupLogsBefore(
  s: ApiSettings,
  before: string,
): Promise<{ deleted: number }> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/logs/cleanup?before=${encodeURIComponent(before)}`,
    s.token,
    { method: "DELETE" },
  );
}

export async function cleanupEventsBefore(
  s: ApiSettings,
  before: string,
): Promise<{ deleted: number }> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/events/cleanup?before=${encodeURIComponent(before)}`,
    s.token,
    { method: "DELETE" },
  );
}

export async function getRecentEvents(
  s: ApiSettings,
  limit: number,
): Promise<RecentEvent[]> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/events/recent?limit=${limit}`,
    s.token,
  );
}

export async function getEvent(
  s: ApiSettings,
  eventId: string,
): Promise<unknown> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/events/${eventId}`, s.token);
}

export async function getActiveSeries(
  s: ApiSettings,
  params: { bucket: "day" | "month"; start?: string; end?: string },
): Promise<ActiveSeriesResponse> {
  const usp = new URLSearchParams();
  usp.set("bucket", params.bucket);
  if (params.start) usp.set("start", params.start);
  if (params.end) usp.set("end", params.end);
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/active?${usp.toString()}`,
    s.token,
  );
}

export async function getDistribution(
  s: ApiSettings,
  params: { dim: "os" | "browser" | "country" | "region" | "city" | "asn_org"; start?: string; end?: string; limit?: number },
): Promise<DistributionResponse> {
  const usp = new URLSearchParams();
  usp.set("dim", params.dim);
  if (params.start) usp.set("start", params.start);
  if (params.end) usp.set("end", params.end);
  usp.set("limit", String(params.limit ?? 10));
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/dist?${usp.toString()}`,
    s.token,
  );
}

export async function getRetention(
  s: ApiSettings,
  params?: { start?: string; end?: string; days?: number[] },
): Promise<RetentionResponse> {
  const usp = new URLSearchParams();
  if (params?.start) usp.set("start", params.start);
  if (params?.end) usp.set("end", params.end);
  if (params?.days && params.days.length > 0) usp.set("days", params.days.join(","));
  const qs = usp.toString();
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/retention${qs ? `?${qs}` : ""}`,
    s.token,
  );
}

export async function getTopEvents(
  s: ApiSettings,
  params?: { start?: string; end?: string; limit?: number; q?: string },
): Promise<TopEventsResponse> {
  const usp = new URLSearchParams();
  if (params?.start) usp.set("start", params.start);
  if (params?.end) usp.set("end", params.end);
  usp.set("limit", String(params?.limit ?? 20));
  if (params?.q) usp.set("q", params.q);
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/events/top?${usp.toString()}`,
    s.token,
  );
}

export async function getFunnel(
  s: ApiSettings,
  params: { steps: string[]; start?: string; end?: string; within?: string },
): Promise<FunnelResponse> {
  const usp = new URLSearchParams();
  usp.set("steps", params.steps.join(","));
  if (params.start) usp.set("start", params.start);
  if (params.end) usp.set("end", params.end);
  if (params.within) usp.set("within", params.within);
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/funnel?${usp.toString()}`,
    s.token,
  );
}

export async function postCustomAnalytics(
  s: ApiSettings,
  body: unknown,
): Promise<CustomAnalyticsResponse> {
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/analytics/custom`,
    s.token,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  );
}

export async function searchLogs(
  s: ApiSettings,
  params: {
    q?: string;
    trace_id?: string;
    level?: string;
    start?: string;
    end?: string;
    limit?: number;
  },
): Promise<LogRow[]> {
  const usp = new URLSearchParams();
  if (params.q) usp.set("q", params.q);
  if (params.trace_id) usp.set("trace_id", params.trace_id);
  if (params.level) usp.set("level", params.level);
  if (params.start) usp.set("start", params.start);
  if (params.end) usp.set("end", params.end);
  usp.set("limit", String(params.limit ?? 100));
  return fetchJSON(
    `${s.apiBase}/api/${s.projectId}/logs/search?${usp.toString()}`,
    s.token,
  );
}

export async function login(
  apiBase: string,
  email: string,
  password: string,
): Promise<LoginResponse> {
  return fetchJSON(`${apiBase}/api/auth/login`, "", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
}

export async function bootstrap(
  apiBase: string,
  email: string,
  password: string,
  projectName: string,
): Promise<BootstrapResponse> {
  return fetchJSON(`${apiBase}/api/auth/bootstrap`, "", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, project_name: projectName }),
  });
}

export async function getSystemStatus(apiBase: string): Promise<SystemStatusResponse> {
  return fetchJSON(`${apiBase}/api/status`, "");
}

export async function getMe(s: ApiSettings): Promise<{ user: User }> {
  return fetchJSON(`${s.apiBase}/api/me`, s.token);
}

export async function listProjects(s: ApiSettings): Promise<{ items: Project[] }> {
  return fetchJSON(`${s.apiBase}/api/projects`, s.token);
}

export async function createProject(
  s: ApiSettings,
  name: string,
): Promise<Project> {
  return fetchJSON(`${s.apiBase}/api/projects`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
}

export async function deleteProject(
  s: ApiSettings,
  projectId: string,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${s.apiBase}/api/projects/${projectId}`, s.token, {
    method: "DELETE",
  });
}

export async function listProjectKeys(
  s: ApiSettings,
  projectId: string,
): Promise<{ items: ProjectKey[] }> {
  return fetchJSON(`${s.apiBase}/api/projects/${projectId}/keys`, s.token);
}

export async function createProjectKey(
  s: ApiSettings,
  projectId: string,
  name: string,
): Promise<ProjectKey> {
  return fetchJSON(`${s.apiBase}/api/projects/${projectId}/keys`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
}

export async function revokeProjectKey(
  s: ApiSettings,
  projectId: string,
  keyId: number,
): Promise<{ revoked: boolean }> {
  return fetchJSON(`${s.apiBase}/api/projects/${projectId}/keys/${keyId}/revoke`, s.token, {
    method: "POST",
  });
}

function alertsBase(s: ApiSettings): string {
  return `${s.apiBase}/api/${s.projectId}/alerts`;
}

function monitorsBase(s: ApiSettings): string {
  return `${s.apiBase}/api/${s.projectId}/monitors`;
}

export async function listAlertContacts(
  s: ApiSettings,
  params?: { type?: "email" | "sms" },
): Promise<{ items: AlertContact[] }> {
  const usp = new URLSearchParams();
  if (params?.type) usp.set("type", params.type);
  const qs = usp.toString();
  return fetchJSON(`${alertsBase(s)}/contacts${qs ? `?${qs}` : ""}`, s.token);
}

export async function createAlertContact(
  s: ApiSettings,
  req: { type: "email" | "sms"; name: string; value: string },
): Promise<AlertContact> {
  return fetchJSON(`${alertsBase(s)}/contacts`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateAlertContact(
  s: ApiSettings,
  contactId: number,
  req: { name?: string; value?: string },
): Promise<AlertContact> {
  return fetchJSON(`${alertsBase(s)}/contacts/${contactId}`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteAlertContact(
  s: ApiSettings,
  contactId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${alertsBase(s)}/contacts/${contactId}`, s.token, {
    method: "DELETE",
  });
}

export async function listAlertContactGroups(
  s: ApiSettings,
  params?: { type?: "email" | "sms" },
): Promise<{ items: AlertContactGroupWithMembers[] }> {
  const usp = new URLSearchParams();
  if (params?.type) usp.set("type", params.type);
  const qs = usp.toString();
  return fetchJSON(`${alertsBase(s)}/contact-groups${qs ? `?${qs}` : ""}`, s.token);
}

export async function createAlertContactGroup(
  s: ApiSettings,
  req: { type: "email" | "sms"; name: string; memberContactIds: number[] },
): Promise<AlertContactGroupUpsertResponse> {
  return fetchJSON(`${alertsBase(s)}/contact-groups`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateAlertContactGroup(
  s: ApiSettings,
  groupId: number,
  req: { name?: string; memberContactIds?: number[] },
): Promise<AlertContactGroupUpsertResponse> {
  return fetchJSON(`${alertsBase(s)}/contact-groups/${groupId}`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteAlertContactGroup(
  s: ApiSettings,
  groupId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${alertsBase(s)}/contact-groups/${groupId}`, s.token, {
    method: "DELETE",
  });
}

export async function listAlertWecomBots(
  s: ApiSettings,
): Promise<{ items: AlertWecomBot[] }> {
  return fetchJSON(`${alertsBase(s)}/wecom-bots`, s.token);
}

export async function createAlertWecomBot(
  s: ApiSettings,
  req: { name: string; webhookUrl: string },
): Promise<AlertWecomBot> {
  return fetchJSON(`${alertsBase(s)}/wecom-bots`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateAlertWecomBot(
  s: ApiSettings,
  botId: number,
  req: { name?: string; webhookUrl?: string },
): Promise<AlertWecomBot> {
  return fetchJSON(`${alertsBase(s)}/wecom-bots/${botId}`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteAlertWecomBot(
  s: ApiSettings,
  botId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${alertsBase(s)}/wecom-bots/${botId}`, s.token, {
    method: "DELETE",
  });
}

export async function listAlertWebhookEndpoints(
  s: ApiSettings,
): Promise<{ items: AlertWebhookEndpoint[] }> {
  return fetchJSON(`${alertsBase(s)}/webhook-endpoints`, s.token);
}

export async function createAlertWebhookEndpoint(
  s: ApiSettings,
  req: { name: string; url: string },
): Promise<AlertWebhookEndpoint> {
  return fetchJSON(`${alertsBase(s)}/webhook-endpoints`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateAlertWebhookEndpoint(
  s: ApiSettings,
  endpointId: number,
  req: { name?: string; url?: string },
): Promise<AlertWebhookEndpoint> {
  return fetchJSON(`${alertsBase(s)}/webhook-endpoints/${endpointId}`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteAlertWebhookEndpoint(
  s: ApiSettings,
  endpointId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${alertsBase(s)}/webhook-endpoints/${endpointId}`, s.token, {
    method: "DELETE",
  });
}

export async function listAlertRules(
  s: ApiSettings,
): Promise<{ items: AlertRule[] }> {
  return fetchJSON(`${alertsBase(s)}/rules`, s.token);
}

export async function createAlertRule(
  s: ApiSettings,
  req: {
    name: string;
    enabled?: boolean;
    source?: AlertRuleSource;
    match?: Record<string, unknown>;
    repeat?: Record<string, unknown>;
    targets?: Record<string, unknown>;
  },
): Promise<AlertRule> {
  return fetchJSON(`${alertsBase(s)}/rules`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateAlertRule(
  s: ApiSettings,
  ruleId: number,
  req: {
    name: string;
    enabled?: boolean;
    source?: AlertRuleSource;
    match?: Record<string, unknown>;
    repeat?: Record<string, unknown>;
    targets?: Record<string, unknown>;
  },
): Promise<AlertRule> {
  return fetchJSON(`${alertsBase(s)}/rules/${ruleId}`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteAlertRule(
  s: ApiSettings,
  ruleId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${alertsBase(s)}/rules/${ruleId}`, s.token, {
    method: "DELETE",
  });
}

export async function testAlertRules(
  s: ApiSettings,
  req: {
    source?: AlertRuleSource;
    level?: string;
    message?: string;
    fields?: Record<string, unknown>;
  },
): Promise<{ items: AlertRulePreview[] }> {
  return fetchJSON(`${alertsBase(s)}/rules/test`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function testAlertRuleDeliveries(
  s: ApiSettings,
  ruleId: number,
  req: {
    source?: AlertRuleSource;
    level?: string;
    message?: string;
    fields?: Record<string, unknown>;
  },
): Promise<{ created: number; items: { id: number; ruleId: number; channelType: string; target: string; title: string }[] }> {
  return fetchJSON(`${alertsBase(s)}/rules/${ruleId}/test-deliveries`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function listAlertDeliveries(
  s: ApiSettings,
  params?: {
    status?: "pending" | "processing" | "sent" | "failed";
    channelType?: "wecom" | "webhook" | "email" | "sms";
    ruleId?: number;
    limit?: number;
  },
): Promise<{ items: AlertDelivery[] }> {
  const usp = new URLSearchParams();
  if (params?.status) usp.set("status", params.status);
  if (params?.channelType) usp.set("channelType", params.channelType);
  if (params?.ruleId && params.ruleId > 0) usp.set("ruleId", String(params.ruleId));
  if (params?.limit && params.limit > 0) usp.set("limit", String(params.limit));
  const qs = usp.toString();
  return fetchJSON(`${alertsBase(s)}/deliveries${qs ? `?${qs}` : ""}`, s.token);
}

export async function listDetectors(
  s: ApiSettings,
): Promise<{ items: DetectorDescriptor[] }> {
  const raw = await fetchJSON<{ items?: unknown[] }>(`${s.apiBase}/api/plugins/detectors`, s.token);
  const items = Array.isArray(raw.items) ? raw.items.map(normalizeDetectorDescriptor).filter((v): v is DetectorDescriptor => Boolean(v)) : [];
  return { items };
}

export async function getDetectorSchema(
  s: ApiSettings,
  detectorType: string,
): Promise<DetectorSchemaResponse> {
  const raw = await fetchJSON<{ detectorType?: unknown; schema?: unknown }>(
    `${s.apiBase}/api/plugins/detectors/${encodeURIComponent(detectorType)}/schema`,
    s.token,
  );
  return {
    detectorType: readString(raw, ["detectorType"]) || detectorType,
    schema: raw.schema ?? {},
  };
}

export async function listMonitors(
  s: ApiSettings,
): Promise<{ items: MonitorDefinition[] }> {
  return fetchJSON(`${monitorsBase(s)}`, s.token);
}

export async function createMonitor(
  s: ApiSettings,
  req: MonitorUpsertRequest,
): Promise<MonitorDefinition> {
  return fetchJSON(`${monitorsBase(s)}`, s.token, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateMonitor(
  s: ApiSettings,
  monitorId: number,
  req: Partial<MonitorUpsertRequest>,
): Promise<MonitorDefinition> {
  return fetchJSON(`${monitorsBase(s)}/${monitorId}`, s.token, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteMonitor(
  s: ApiSettings,
  monitorId: number,
): Promise<{ deleted: boolean }> {
  return fetchJSON(`${monitorsBase(s)}/${monitorId}`, s.token, {
    method: "DELETE",
  });
}

export async function runMonitorNow(
  s: ApiSettings,
  monitorId: number,
): Promise<{ queued: boolean }> {
  return fetchJSON(`${monitorsBase(s)}/${monitorId}/run`, s.token, {
    method: "POST",
  });
}

export async function testMonitor(
  s: ApiSettings,
  monitorId: number,
): Promise<MonitorTestResult> {
  return fetchJSON(`${monitorsBase(s)}/${monitorId}/test`, s.token, {
    method: "POST",
  });
}

export async function listMonitorRuns(
  s: ApiSettings,
  monitorId: number,
  params?: { limit?: number },
): Promise<{ items: MonitorRun[] }> {
  const usp = new URLSearchParams();
  if (params?.limit && params.limit > 0) usp.set("limit", String(params.limit));
  const qs = usp.toString();
  return fetchJSON(`${monitorsBase(s)}/${monitorId}/runs${qs ? `?${qs}` : ""}`, s.token);
}

function normalizeDetectorDescriptor(raw: unknown): DetectorDescriptor | null {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  const rec = raw as Record<string, unknown>;
  const type = readString(rec, ["type", "Type"]).toLowerCase();
  if (!type) return null;
  return {
    type,
    mode: readString(rec, ["mode", "Mode"]),
    path: readString(rec, ["path", "Path"]),
  };
}

function readString(rec: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = rec[key];
    if (typeof value === "string") return value.trim();
  }
  return "";
}

function handleUnauthorized() {
  if (typeof window === "undefined") return;
  try {
    clearAuth();
  } catch {
  }
  if (window.location.pathname !== "/login") {
    window.location.href = "/login";
  }
}

async function fetchJSON<T>(
  url: string,
  token?: string,
  init?: RequestInit,
): Promise<T> {
  const headers: Record<string, string> = {
    Accept: "application/json",
    ...(init?.headers ? (init.headers as Record<string, string>) : {}),
  };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(url, { ...init, headers });
  if (res.status === 401 && token && !url.includes("/api/auth/")) {
    handleUnauthorized();
  }
  const text = await res.text().catch(() => "");
  const contentType = res.headers.get("content-type") || "";
  const isJSON = contentType.includes("application/json");

  const parseJSON = () => {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return undefined;
    }
  };

  const payload = isJSON ? parseJSON() : undefined;
  const env =
    payload && typeof payload === "object" && payload !== null && "code" in payload
      ? (payload as ApiEnvelope<T>)
      : undefined;

  if (!res.ok) {
    const msg = env?.err || (typeof payload === "object" && payload && "err" in payload ? String((payload as any).err) : "") || text;
    throw new Error(`${res.status} ${res.statusText}${msg ? `: ${msg}` : ""}`);
  }

  if (env) {
    if (env.code !== 0) {
      throw new Error(env.err || `code ${env.code}`);
    }
    return env.data as T;
  }

  return (isJSON ? (payload as T) : (text as unknown as T));
}
