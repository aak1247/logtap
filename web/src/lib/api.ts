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
  events: number;
  errors: number;
  users: number;
};

export type User = { id: number; email: string };
export type LoginResponse = { token: string; user: User };
export type BootstrapResponse = {
  token: string;
  user: User;
  project: { id: number; name: string };
  key: { id: number; name: string; key: string };
};

export type SystemStatusResponse = {
  status: "uninitialized" | "running" | "maintenance" | "exception";
  initialized: boolean;
  auth_enabled: boolean;
  message?: string;
};

export type Project = { id: number; owner_user_id: number; name: string };
export type ProjectKey = {
  id: number;
  project_id: number;
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

export async function listProjectKeys(
  s: ApiSettings,
  projectId: number,
): Promise<{ items: ProjectKey[] }> {
  return fetchJSON(`${s.apiBase}/api/projects/${projectId}/keys`, s.token);
}

export async function createProjectKey(
  s: ApiSettings,
  projectId: number,
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
  projectId: number,
  keyId: number,
): Promise<{ revoked: boolean }> {
  return fetchJSON(`${s.apiBase}/api/projects/${projectId}/keys/${keyId}/revoke`, s.token, {
    method: "POST",
  });
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
