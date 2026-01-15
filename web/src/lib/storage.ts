type Settings = {
  apiBase: string;
  token: string;
  projectId: string;
};

const key = "logtap:settings:v1";
const settingsChangedEvent = "logtap:settings-changed";

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
    return { apiBase: fallbackApiBase, token: "", projectId };
  }

  try {
    const raw = localStorage.getItem(key);
    if (!raw)
      return {
        apiBase: fallbackApiBase,
        token: "",
        projectId,
      };
    const parsed = JSON.parse(raw) as Partial<Settings>;
    return {
      apiBase: parsed.apiBase || fallbackApiBase,
      token: parsed.token || "",
      projectId: parsed.projectId || projectId,
    };
  } catch {
    return { apiBase: fallbackApiBase, token: "", projectId };
  }
}

function notifySettingsChanged() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(settingsChangedEvent));
}

export function saveSettings(next: Settings) {
  try {
    localStorage.setItem(key, JSON.stringify(next));
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
