import { loadSettings } from "./storage";

type SelfLogReq = {
  level?: string;
  message: string;
  fields?: Record<string, unknown>;
};

let inited = false;
let lastKey = "";
let lastAt = 0;

async function postSelfLog(req: SelfLogReq) {
  const s = loadSettings();
  if (!s.apiBase || !s.selfLogProjectId || !s.selfLogProjectKey) return;
  try {
    await fetch(`${s.apiBase}/api/${encodeURIComponent(s.selfLogProjectId)}/logs/`, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "X-Project-Key": s.selfLogProjectKey,
      },
      body: JSON.stringify(req),
      keepalive: true,
    });
  } catch {
  }
}

function shouldSend(key: string): boolean {
  const now = Date.now();
  if (key && key === lastKey && now-lastAt < 5000) return false;
  if (now - lastAt < 800) return false;
  lastKey = key;
  lastAt = now;
  return true;
}

export function initSelfLogging() {
  if (inited) return;
  inited = true;

  window.addEventListener("error", (ev) => {
    const err = (ev as ErrorEvent).error as unknown;
    const msg =
      err instanceof Error
        ? err.message
        : (ev as ErrorEvent).message || "window.error";
    const stack = err instanceof Error ? err.stack : undefined;
    const key = `error:${msg}:${stack ?? ""}`;
    if (!shouldSend(key)) return;
    void postSelfLog({
      level: "error",
      message: msg,
      fields: {
        kind: "window.error",
        href: window.location.href,
        ua: navigator.userAgent,
        filename: (ev as ErrorEvent).filename,
        lineno: (ev as ErrorEvent).lineno,
        colno: (ev as ErrorEvent).colno,
        stack,
      },
    });
  });

  window.addEventListener("unhandledrejection", (ev) => {
    const r = (ev as PromiseRejectionEvent).reason as unknown;
    const msg = r instanceof Error ? r.message : String(r);
    const stack = r instanceof Error ? r.stack : undefined;
    const key = `rejection:${msg}:${stack ?? ""}`;
    if (!shouldSend(key)) return;
    void postSelfLog({
      level: "error",
      message: msg,
      fields: {
        kind: "unhandledrejection",
        href: window.location.href,
        ua: navigator.userAgent,
        stack,
      },
    });
  });
}
