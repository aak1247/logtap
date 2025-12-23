// Demo uses local SDK path; in real apps use `import { LogtapClient } from "logtap-sdk"`.
import { LogtapClient } from "../../sdks/js/logtap/index.js";

const $ = (id) => /** @type {HTMLElement} */ (document.getElementById(id));

const out = $("out");

/** @type {LogtapClient|null} */
let client = null;

function logLine(line) {
  const ts = new Date().toISOString();
  out.textContent = `[${ts}] ${line}\n` + (out.textContent || "");
}

function loadConfig() {
  try {
    const raw = localStorage.getItem("logtap_demo_cfg");
    if (!raw) return null;
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function saveConfig(cfg) {
  try {
    localStorage.setItem("logtap_demo_cfg", JSON.stringify(cfg));
  } catch {}
}

function setEnabled(enabled) {
  for (const id of ["flush", "sendInfo", "sendError", "sendTrack", "throw", "reject"]) {
    /** @type {HTMLButtonElement} */ ($(`${id}`)).disabled = !enabled;
  }
}

function readUI() {
  /** @type {HTMLInputElement} */
  const baseUrl = $("baseUrl");
  /** @type {HTMLInputElement} */
  const projectId = $("projectId");
  /** @type {HTMLInputElement} */
  const projectKey = $("projectKey");
  /** @type {HTMLInputElement} */
  const gzip = $("gzip");

  return {
    baseUrl: baseUrl.value.trim() || "http://localhost:8080",
    projectId: projectId.value.trim() || "1",
    projectKey: projectKey.value.trim(),
    gzip: Boolean(gzip.checked),
  };
}

function writeUI(cfg) {
  /** @type {HTMLInputElement} */
  const baseUrl = $("baseUrl");
  /** @type {HTMLInputElement} */
  const projectId = $("projectId");
  /** @type {HTMLInputElement} */
  const projectKey = $("projectKey");
  /** @type {HTMLInputElement} */
  const gzip = $("gzip");

  baseUrl.value = cfg.baseUrl ?? "http://localhost:8080";
  projectId.value = String(cfg.projectId ?? "1");
  projectKey.value = cfg.projectKey ?? "";
  gzip.checked = Boolean(cfg.gzip ?? true);
}

const existing = loadConfig();
if (existing) writeUI(existing);
else writeUI({ baseUrl: "http://localhost:8080", projectId: "1", projectKey: "", gzip: true });

$("init").addEventListener("click", async () => {
  const cfg = readUI();
  saveConfig(cfg);

  // Create a client once after user config is ready.
  client = new LogtapClient({
    baseUrl: cfg.baseUrl,
    projectId: cfg.projectId,
    projectKey: cfg.projectKey || undefined,
    gzip: cfg.gzip,
    globalTags: { env: "demo", runtime: "browser" },
    globalContexts: { demo: { page: location.pathname, ua: navigator.userAgent } },
  });

  // Optional: capture window.error + unhandledrejection.
  client.captureBrowserErrors();

  setEnabled(true);
  logLine("client initialized");

  client.identify("u_demo_browser", { plan: "free" });
  client.info("browser demo init", { url: location.href });
  client.track("demo_init", { kind: "browser" });

  // Try a first flush to verify connectivity quickly (optional).
  await client.flush();
  logLine("flush ok");
});

$("flush").addEventListener("click", async () => {
  if (!client) return;
  await client.flush();
  logLine("flush ok");
});

$("sendInfo").addEventListener("click", () => {
  if (!client) return;
  client.info("hello from browser", { k: "v", t: Date.now() }, { tags: { btn: "info" }, contexts: { page: { href: location.href } } });
  logLine("queued info log");
});

$("sendError").addEventListener("click", () => {
  if (!client) return;
  client.error("boom from browser", { err: "demo_error", stack: "n/a" }, { tags: { btn: "error" } });
  logLine("queued error log");
});

$("sendTrack").addEventListener("click", () => {
  if (!client) return;
  client.track("signup", { from: "browser_demo", ts: Date.now() }, { tags: { btn: "track" } });
  logLine("queued track event");
});

$("throw").addEventListener("click", () => {
  setTimeout(() => {
    throw new Error("demo window.error");
  }, 0);
  logLine("scheduled throw (window.error)");
});

$("reject").addEventListener("click", () => {
  Promise.reject(new Error("demo unhandledrejection"));
  logLine("created Promise rejection");
});

setEnabled(false);
