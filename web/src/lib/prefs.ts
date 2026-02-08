function clampInt(n: number, min: number, max: number): number {
  if (!Number.isFinite(n)) return min;
  const v = Math.floor(n);
  if (v < min) return min;
  if (v > max) return max;
  return v;
}

function getRuntimeOrigin(): string {
  if (typeof window === "undefined") return "";
  const origin = window.location.origin;
  if (origin && origin !== "null") return origin;
  const protocol = window.location.protocol;
  const host = window.location.host;
  if (!protocol || !host) return "";
  return `${protocol}//${host}`;
}

function storageKey(): string {
  const base = "logtap:prefs:v1";
  const origin = getRuntimeOrigin();
  return origin ? `${base}:${origin}` : base;
}

export function clampFunnelDays(n: number): number {
  return clampInt(n, 1, 31);
}

export function loadFunnelDays(): number {
  if (typeof window === "undefined") return 7;
  try {
    const raw = localStorage.getItem(storageKey());
    if (!raw) return 7;
    const parsed = JSON.parse(raw) as { funnelDays?: unknown };
    const n = typeof parsed.funnelDays === "number" ? parsed.funnelDays : Number(parsed.funnelDays);
    return clampFunnelDays(n);
  } catch {
    return 7;
  }
}

export function saveFunnelDays(days: number) {
  if (typeof window === "undefined") return;
  try {
    const key = storageKey();
    const raw = localStorage.getItem(key);
    const parsed = raw ? (JSON.parse(raw) as Record<string, unknown>) : {};
    parsed.funnelDays = clampFunnelDays(days);
    localStorage.setItem(key, JSON.stringify(parsed));
  } catch {
    // ignore
  }
}

