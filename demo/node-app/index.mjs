// Demo uses local SDK path; in real apps use `import { LogtapClient } from "logtap-sdk"`.
import { LogtapClient } from "../../sdks/js/logtap/index.js";

// Env config (projectKey is only needed when AUTH_SECRET is enabled).
const baseUrl = (process.env.LOGTAP_BASE_URL || "http://localhost:8080").trim();
const projectId = (process.env.LOGTAP_PROJECT_ID || "1").trim();
const projectKey = (process.env.LOGTAP_PROJECT_KEY || "").trim();
const gzip = String(process.env.LOGTAP_GZIP || "true").toLowerCase() !== "false";
const durationMs = Number(process.env.DEMO_DURATION_MS || "0") || 0;

// Create one client and reuse it across the process.
const client = new LogtapClient({
  baseUrl,
  projectId,
  projectKey: projectKey || undefined,
  gzip,
  globalTags: { env: "demo", runtime: "node" },
  globalContexts: { demo: { kind: "node" } },
});

// Optional: capture unhandled exceptions once per process.
client.captureNodeErrors();
client.identify("u_demo_node", { plan: "free" }); // Optional: set a user profile.

client.info("node demo start", {
  pid: process.pid,
  node: process.version,
  argv: process.argv,
});
client.track("demo_init", { kind: "node" });

let n = 0;
const ticker = setInterval(() => {
  n += 1;
  const mem = process.memoryUsage();
  client.info("node heartbeat", {
    n,
    uptime_s: Math.round(process.uptime()),
    rss_mb: Math.round(mem.rss / 1024 / 1024),
    heap_used_mb: Math.round(mem.heapUsed / 1024 / 1024),
  });
  if (n % 5 === 0) {
    client.track("heartbeat", { n });
  }
}, 1000);

// Flush before exit (demo uses process.exit; apps can just return and let the process shut down).
async function shutdown(signal) {
  try {
    clearInterval(ticker);
    client.warn("node demo shutdown", { signal, n });
    await client.close();
  } finally {
    process.exit(0);
  }
}

process.on("SIGINT", () => void shutdown("SIGINT"));
process.on("SIGTERM", () => void shutdown("SIGTERM"));

if (durationMs > 0) {
  setTimeout(() => void shutdown(`DEMO_DURATION_MS=${durationMs}`), durationMs).unref?.();
}

console.log("logtap node demo running");
console.log("  baseUrl   =", baseUrl);
console.log("  projectId =", projectId);
console.log("  gzip      =", gzip);
if (durationMs > 0) console.log("  duration  =", durationMs, "ms");
console.log("Press Ctrl+C to stop");
