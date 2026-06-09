import http from "k6/http";
import { check, fail, sleep } from "k6";
import { Counter, Rate } from "k6/metrics";
import exec from "k6/execution";

const BASE_URL = trimSlash(__ENV.BASE_URL || "http://127.0.0.1:8080");
const PROJECT_ID = __ENV.PROJECT_ID || "1";
const PROJECT_KEY = __ENV.PROJECT_KEY || "";
const AUTH_TOKEN = __ENV.AUTH_TOKEN || "";
const PROXY_SECRET = __ENV.PROXY_SECRET || "";
const TARGET_LOGS = parseInt(__ENV.TARGET_LOGS || "1000000", 10);
const BATCH_SIZE = parseInt(__ENV.BATCH_SIZE || "100", 10);
const QUERY_DURATION = __ENV.QUERY_DURATION || "5m";
const QUERY_VUS = parseInt(__ENV.QUERY_VUS || "20", 10);
const INGEST_VUS = parseInt(__ENV.INGEST_VUS || "50", 10);
const START_OFFSET_DAYS = parseInt(__ENV.START_OFFSET_DAYS || "29", 10);
const RUN_ID = __ENV.RUN_ID || `${Date.now()}`;
const QUERY_SLEEP_MS = parseInt(__ENV.QUERY_SLEEP_MS || "100", 10);
const INGEST_SLEEP_MS = parseInt(__ENV.INGEST_SLEEP_MS || "0", 10);
const EXPECT_QUERY_OK = (__ENV.EXPECT_QUERY_OK || "true").toLowerCase() !== "false";
const INCLUDE_TRACK = (__ENV.INCLUDE_TRACK || "false").toLowerCase() === "true";
const INCLUDE_REDIS_ANALYTICS = (__ENV.INCLUDE_REDIS_ANALYTICS || "false").toLowerCase() === "true";

const batches = Math.ceil(TARGET_LOGS / BATCH_SIZE);
const startTime = new Date(Date.now() - START_OFFSET_DAYS * 24 * 60 * 60 * 1000);
const endTime = new Date(Date.now() + 60 * 1000);
const startISO = startTime.toISOString();
const endISO = endTime.toISOString();
const queryHeaders = buildQueryHeaders();
const ingestHeaders = buildIngestHeaders();
const queryErrorRate = new Rate("query_errors");
const ingestErrorRate = new Rate("ingest_errors");
const ingestedLogs = new Counter("ingested_logs");
const ingestedTrackEvents = new Counter("ingested_track_events");

export const options = {
  scenarios: {
    ingest_logs: {
      executor: "shared-iterations",
      vus: INGEST_VUS,
      iterations: batches,
      maxDuration: __ENV.INGEST_MAX_DURATION || "30m",
      exec: "ingestLogs",
    },
    query_common: {
      executor: "constant-vus",
      vus: QUERY_VUS,
      duration: QUERY_DURATION,
      startTime: __ENV.QUERY_START_TIME || "15s",
      exec: "queryCommon",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.05"],
    ingest_errors: ["rate<0.01"],
    query_errors: ["rate<0.05"],
    http_req_duration: ["p(95)<2000", "p(99)<5000"],
  },
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
};

export function setup() {
  if (!BASE_URL || !PROJECT_ID) {
    fail("BASE_URL and PROJECT_ID are required");
  }
  if (!PROJECT_KEY && !PROXY_SECRET) {
    console.warn("PROJECT_KEY/PROXY_SECRET is empty; ingest only works when auth/project-key is disabled");
  }
  if (!AUTH_TOKEN && !PROXY_SECRET) {
    console.warn("AUTH_TOKEN/PROXY_SECRET is empty; query only works when auth is disabled");
  }
  console.log(
    JSON.stringify({
      base_url: BASE_URL,
      project_id: PROJECT_ID,
      run_id: RUN_ID,
      target_logs: TARGET_LOGS,
      batch_size: BATCH_SIZE,
      ingest_batches: batches,
      ingest_vus: INGEST_VUS,
      query_vus: QUERY_VUS,
      query_duration: QUERY_DURATION,
      query_window: { start: startISO, end: endISO },
      include_track: INCLUDE_TRACK,
      include_redis_analytics: INCLUDE_REDIS_ANALYTICS,
    }),
  );
}

export function ingestLogs() {
  const batchIndex = exec.scenario.iterationInTest;
  const remaining = TARGET_LOGS - batchIndex * BATCH_SIZE;
  if (remaining <= 0) return;
  const size = Math.min(BATCH_SIZE, remaining);
  const logs = [];

  for (let i = 0; i < size; i++) {
    const seq = batchIndex * BATCH_SIZE + i;
    const ts = timestampFor(seq);
    const level = levelFor(seq);
    const eventName = eventNameFor(seq);
    logs.push({
      level,
      message: level === "event" ? eventName : `k6-rollup-${RUN_ID} log ${seq}`,
      device_id: `device-${seq % 50000}`,
      trace_id: `trace-${RUN_ID}-${seq % 200000}`,
      span_id: `span-${seq % 1000}`,
      timestamp: ts,
      fields: {
        source: "k6-rollup-pressure",
        run_id: RUN_ID,
        seq,
        event_name: eventName,
        route: routeFor(seq),
        service: serviceFor(seq),
        status: seq % 17 === 0 ? 500 : 200,
        browser: browserFor(seq),
        os: osFor(seq),
        country: countryFor(seq),
      },
      user: {
        id: `user-${seq % 200000}`,
        username: `user-${seq % 200000}`,
      },
      tags: {
        env: "bench",
        run_id: RUN_ID,
      },
    });
  }

  const res = http.post(`${BASE_URL}/api/${PROJECT_ID}/logs/`, JSON.stringify(logs), {
    headers: ingestHeaders,
    timeout: "30s",
    tags: { endpoint: "ingest_logs" },
  });
  const ok = check(res, { "ingest logs status 202": (r) => r.status === 202 });
  ingestErrorRate.add(!ok);
  if (ok) ingestedLogs.add(size);

  if (INCLUDE_TRACK) {
    const trackItems = [];
    const trackSize = Math.max(1, Math.floor(size / 10));
    for (let i = 0; i < trackSize; i++) {
      const seq = batchIndex * BATCH_SIZE + i;
      trackItems.push({
        name: eventNameFor(seq),
        device_id: `device-${seq % 50000}`,
        timestamp: timestampFor(seq),
        properties: {
          source: "k6-rollup-pressure",
          run_id: RUN_ID,
          seq,
          route: routeFor(seq),
          browser: browserFor(seq),
          os: osFor(seq),
          country: countryFor(seq),
        },
        user: { id: `user-${seq % 200000}` },
      });
    }
    const trackRes = http.post(`${BASE_URL}/api/${PROJECT_ID}/track/`, JSON.stringify(trackItems), {
      headers: ingestHeaders,
      timeout: "30s",
      tags: { endpoint: "ingest_track" },
    });
    const trackOK = check(trackRes, { "ingest track status 202": (r) => r.status === 202 });
    ingestErrorRate.add(!trackOK);
    if (trackOK) ingestedTrackEvents.add(trackSize);
  }

  if (INGEST_SLEEP_MS > 0) sleep(INGEST_SLEEP_MS / 1000);
}

export function queryCommon() {
  const endpoints = [
    ["metrics_today", "GET", `/api/${PROJECT_ID}/metrics/today`],
    ["metrics_total", "GET", `/api/${PROJECT_ID}/metrics/total`],
    ["logs_recent", "GET", `/api/${PROJECT_ID}/logs/search?limit=100&start=${enc(startISO)}&end=${enc(endISO)}`],
    ["logs_error", "GET", `/api/${PROJECT_ID}/logs/search?level=error&limit=100&start=${enc(startISO)}&end=${enc(endISO)}`],
    ["logs_fts", "GET", `/api/${PROJECT_ID}/logs/search?q=${enc("k6-rollup")}&mode=fts&limit=100&start=${enc(startISO)}&end=${enc(endISO)}`],
    ["events_recent", "GET", `/api/${PROJECT_ID}/events/recent?limit=100`],
    ["events_top", "GET", `/api/${PROJECT_ID}/analytics/events/top?limit=20&start=${enc(startISO)}&end=${enc(endISO)}`],
    ["users_growth", "GET", `/api/${PROJECT_ID}/analytics/users?start=${enc(startISO)}&end=${enc(endISO)}`],
    ["funnel", "GET", `/api/${PROJECT_ID}/analytics/funnel?steps=${enc("signup,view_item,purchase")}&within=24h&source=logs&start=${enc(startISO)}&end=${enc(endISO)}`],
    ["custom_events", "POST", `/api/${PROJECT_ID}/analytics/custom`, customAnalyticsPayload("count_events")],
    ["custom_users", "POST", `/api/${PROJECT_ID}/analytics/custom`, customAnalyticsPayload("count_users")],
    ["storage_estimate", "GET", `/api/${PROJECT_ID}/storage/estimate`],
    ["properties_schema", "GET", `/api/${PROJECT_ID}/properties/schema`],
    ["events_schema", "GET", `/api/${PROJECT_ID}/events/schema`],
    ["cleanup_policy", "GET", `/api/${PROJECT_ID}/cleanup/policy`],
    ["analytics_views", "GET", `/api/${PROJECT_ID}/analytics/views?limit=50`],
    ["alerts_contacts", "GET", `/api/${PROJECT_ID}/alerts/contacts?limit=50`],
    ["alerts_groups", "GET", `/api/${PROJECT_ID}/alerts/contact-groups?limit=50`],
    ["alerts_channels", "GET", `/api/${PROJECT_ID}/alerts/webhook-endpoints`],
    ["alerts_rules", "GET", `/api/${PROJECT_ID}/alerts/rules`],
    ["alerts_deliveries", "GET", `/api/${PROJECT_ID}/alerts/deliveries?limit=50`],
    ["monitors", "GET", `/api/${PROJECT_ID}/monitors`],
  ];
  if (INCLUDE_REDIS_ANALYTICS) {
    endpoints.push(
      ["analytics_active", "GET", `/api/${PROJECT_ID}/analytics/active?bucket=day&start=${enc(startISO)}&end=${enc(endISO)}`],
      ["analytics_dist_browser", "GET", `/api/${PROJECT_ID}/analytics/dist?dim=browser&limit=10&start=${enc(startISO)}&end=${enc(endISO)}`],
      ["analytics_retention", "GET", `/api/${PROJECT_ID}/analytics/retention?days=1,7,30&start=${enc(startISO)}&end=${enc(endISO)}`],
    );
  }

  const index = exec.scenario.iterationInTest % endpoints.length;
  const [name, method, path, body] = endpoints[index];
  const params = {
    headers: queryHeaders,
    timeout: "30s",
    tags: { endpoint: name },
  };

  let res;
  if (method === "POST") {
    res = http.post(`${BASE_URL}${path}`, JSON.stringify(body), params);
  } else {
    res = http.get(`${BASE_URL}${path}`, params);
  }

  const ok = check(res, {
    [`${name} status ok`]: (r) => EXPECT_QUERY_OK ? r.status >= 200 && r.status < 300 : r.status < 500,
  });
  queryErrorRate.add(!ok);
  if (!ok) {
    console.warn(`${name} status=${res.status} body=${String(res.body).slice(0, 300)}`);
  }

  if (QUERY_SLEEP_MS > 0) sleep(QUERY_SLEEP_MS / 1000);
}

function customAnalyticsPayload(metricType) {
  return {
    analysis_type: "event",
    time_range: {
      start: startISO,
      end: endISO,
      granularity: "day",
    },
    target: {
      events: ["signup", "view_item", "purchase", "error_seen"],
    },
    metric: {
      type: metricType,
    },
    group_by: ["time", "event"],
    filter: {
      events: ["signup", "view_item", "purchase", "error_seen"],
      properties: {},
    },
  };
}

function buildIngestHeaders() {
  const headers = { "Content-Type": "application/json" };
  if (PROJECT_KEY) headers["X-Project-Key"] = PROJECT_KEY;
  if (PROXY_SECRET) headers["X-Logtap-Proxy-Secret"] = PROXY_SECRET;
  return headers;
}

function buildQueryHeaders() {
  const headers = { "Content-Type": "application/json" };
  if (AUTH_TOKEN) headers.Authorization = `Bearer ${AUTH_TOKEN}`;
  if (PROXY_SECRET) headers["X-Logtap-Proxy-Secret"] = PROXY_SECRET;
  return headers;
}

function timestampFor(seq) {
  const spanMs = Math.max(1, endTime.getTime() - startTime.getTime());
  const offsetMs = Math.floor((seq / Math.max(1, TARGET_LOGS)) * spanMs);
  return new Date(startTime.getTime() + Math.min(offsetMs, spanMs - 1)).toISOString();
}

function levelFor(seq) {
  if (seq % 10 === 0) return "event";
  if (seq % 37 === 0) return "fatal";
  if (seq % 11 === 0) return "error";
  if (seq % 7 === 0) return "warn";
  return "info";
}

function eventNameFor(seq) {
  const names = ["signup", "view_item", "add_to_cart", "purchase", "error_seen", "page_view"];
  return names[seq % names.length];
}

function routeFor(seq) {
  const routes = ["/", "/pricing", "/docs", "/checkout", "/api/search", "/api/events"];
  return routes[seq % routes.length];
}

function serviceFor(seq) {
  const services = ["gateway", "worker", "consumer", "web", "api"];
  return services[seq % services.length];
}

function browserFor(seq) {
  const browsers = ["Chrome", "Safari", "Firefox", "Edge"];
  return browsers[seq % browsers.length];
}

function osFor(seq) {
  const oses = ["macOS", "Windows", "Linux", "iOS", "Android"];
  return oses[seq % oses.length];
}

function countryFor(seq) {
  const countries = ["CN", "US", "SG", "JP", "DE"];
  return countries[seq % countries.length];
}

function trimSlash(value) {
  return String(value).replace(/\/+$/, "");
}

function enc(value) {
  return encodeURIComponent(value);
}
