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
export type LoginResponse = { token: string; user: User };
export type BootstrapResponse = {
  token: string;
  user: User;
  project: { id: string; name: string };
  key: { id: number; name: string; key: string };
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

export async function getMetricsToday(s: ApiSettings): Promise<MetricsToday> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/metrics/today`, s.token);
}

export async function getMetricsTotal(s: ApiSettings): Promise<MetricsTotal> {
  return fetchJSON(`${s.apiBase}/api/${s.projectId}/metrics/total`, s.token);
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
