type Settings = {
  apiBase: string;
  token: string;
  projectId: string;
};

const legacyKey = "logtap:settings:v1";
const settingsChangedEvent = "logtap:settings-changed";

let cachedSettings: Settings | null = null;

export function normalizeApiBase(raw: string): string {
  let base = raw.trim();
  if (base && !base.includes("://") && !base.startsWith("/")) {
    base = `http://${base}`;
  }
  base = base.replace(/\/+$/, "");
  base = base.replace(/\/api\/?$/, "");
  base = base.replace(/\/+$/, "");
  return base;
}

function sameSettings(a: Settings, b: Settings): boolean {
  return a.apiBase === b.apiBase && a.token === b.token && a.projectId === b.projectId;
}

function getRuntimeApiBase(): string {
  if (typeof window === "undefined") return "";
  const origin = window.location.origin;
  if (origin && origin !== "null") return origin;
  const protocol = window.location.protocol;
  const host = window.location.host;
  if (!protocol || !host) return "";
  return `${protocol}//${host}`;
}

function getSettingsStorageKey(): string {
  const configured = (import.meta.env.VITE_SETTINGS_STORAGE_KEY as string | undefined) ?? "";
  const base = configured.trim() || legacyKey;
  const runtime = getRuntimeApiBase();
  if (!runtime) return base;
  return `${base}:${runtime}`;
}

export function loadSettings(): Settings {
  const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
  const projectId =
    (import.meta.env.VITE_DEFAULT_PROJECT_ID as string | undefined) ?? "";
  const runtimeApiBase = getRuntimeApiBase();
  const fallbackApiBase = normalizeApiBase(runtimeApiBase || apiBase || "http://localhost:8080");

  if (typeof window === "undefined") {
    const next = { apiBase: fallbackApiBase, token: "", projectId };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  }

  try {
    const key = getSettingsStorageKey();
    const raw = localStorage.getItem(key) ?? localStorage.getItem(legacyKey);
    if (!raw) {
      const next = {
        apiBase: fallbackApiBase,
        token: "",
        projectId,
      };
      if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
      cachedSettings = next;
      return next;
    }
    const parsed = JSON.parse(raw) as Partial<Settings>;
    const next = {
      apiBase: normalizeApiBase(parsed.apiBase || fallbackApiBase),
      token: parsed.token || "",
      projectId: parsed.projectId || projectId,
    };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  } catch {
    const next = { apiBase: fallbackApiBase, token: "", projectId };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  }
}

function notifySettingsChanged() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(settingsChangedEvent));
}

export function saveSettings(next: Settings) {
  try {
    const normalized = {
      ...next,
      apiBase: normalizeApiBase(next.apiBase),
    };
    const key = getSettingsStorageKey();
    localStorage.setItem(key, JSON.stringify(normalized));
    cachedSettings = normalized;
  } finally {
    notifySettingsChanged();
  }
}

export function clearAuth() {
  const s = loadSettings();
  saveSettings({ apiBase: s.apiBase, token: "", projectId: "" });
}

export function subscribeSettingsChange(listener: () => void): () => void {
  if (typeof window === "undefined") return () => {};
  const key = getSettingsStorageKey();

  const onEvent: EventListener = () => listener();
  const onStorage = (e: StorageEvent) => {
    if (e.key === key || e.key === legacyKey) listener();
  };

  window.addEventListener(settingsChangedEvent, onEvent);
  window.addEventListener("storage", onStorage);

  return () => {
    window.removeEventListener(settingsChangedEvent, onEvent);
    window.removeEventListener("storage", onStorage);
  };
}
