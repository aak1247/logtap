type Settings = {
  apiBase: string;
  token: string;
  projectId: string;
};

const key = "logtap:settings:v1";
const settingsChangedEvent = "logtap:settings-changed";

let cachedSettings: Settings | null = null;

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

export function loadSettings(): Settings {
  const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
  const projectId =
    (import.meta.env.VITE_DEFAULT_PROJECT_ID as string | undefined) ?? "";
  const runtimeApiBase = getRuntimeApiBase();
  const fallbackApiBase = runtimeApiBase || apiBase || "http://localhost:8080";

  if (typeof window === "undefined") {
    const next = { apiBase: fallbackApiBase, token: "", projectId };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  }

  try {
    const raw = localStorage.getItem(key);
    if (!raw)
      {
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
      apiBase: parsed.apiBase || fallbackApiBase,
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
    localStorage.setItem(key, JSON.stringify(next));
    cachedSettings = next;
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

  const onEvent: EventListener = () => listener();
  const onStorage = (e: StorageEvent) => {
    if (e.key === key) listener();
  };

  window.addEventListener(settingsChangedEvent, onEvent);
  window.addEventListener("storage", onStorage);

  return () => {
    window.removeEventListener(settingsChangedEvent, onEvent);
    window.removeEventListener("storage", onStorage);
  };
}
