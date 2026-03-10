type Settings = {
  apiBase: string;
  token: string;
  projectId: string;
  selfLogProjectId: string;
  selfLogProjectKey: string;
};

const settingsStorageKey = "logtap:settings:v1";
const settingsChangedEvent = "logtap:settings-changed";

let cachedSettings: Settings | null = null;

function envBool(value: unknown): boolean {
  if (typeof value !== "string") return false;
  switch (value.trim().toLowerCase()) {
    case "1":
    case "true":
    case "yes":
    case "on":
      return true;
    default:
      return false;
  }
}

export type ApiBaseLockMode = "off" | "once" | "always";

export function getApiBaseLockMode(): ApiBaseLockMode {
  const raw = ((import.meta.env.VITE_LOCK_API_BASE as string | undefined) ?? "")
    .trim()
    .toLowerCase();
  if (!raw) return "off";
  if (raw === "always") return "always";
  if (raw === "once" || envBool(raw)) return "once";
  return "off";
}

export function isApiBaseLocked(): boolean {
  return getApiBaseLockMode() !== "off";
}

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
  return (
    a.apiBase === b.apiBase &&
    a.token === b.token &&
    a.projectId === b.projectId &&
    a.selfLogProjectId === b.selfLogProjectId &&
    a.selfLogProjectKey === b.selfLogProjectKey
  );
}

function getSettingsStorageKey(): string {
  const configured = (import.meta.env.VITE_SETTINGS_STORAGE_KEY as string | undefined) ?? "";
  return configured.trim() || settingsStorageKey;
}

function readStoredSettingsRaw(): string | null {
  if (typeof window === "undefined") return null;
  const key = getSettingsStorageKey();
  return localStorage.getItem(key);
}

export function canEditApiBase(): boolean {
  const mode = getApiBaseLockMode();
  if (mode === "off") return true;
  if (mode === "always") return false;
  const raw = readStoredSettingsRaw();
  if (!raw) return true;
  try {
    const parsed = JSON.parse(raw) as Partial<Settings>;
    return !parsed.apiBase;
  } catch {
    return true;
  }
}

export function loadSettings(): Settings {
  const lockMode = getApiBaseLockMode();
  const envApiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
  const projectId =
    (import.meta.env.VITE_DEFAULT_PROJECT_ID as string | undefined) ?? "";
  const fallbackApiBase = normalizeApiBase(envApiBase || "http://localhost:8080");

  if (typeof window === "undefined") {
    const next = { apiBase: fallbackApiBase, token: "", projectId, selfLogProjectId: "", selfLogProjectKey: "" };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  }

  try {
    const raw = readStoredSettingsRaw();
    if (!raw) {
      const next = {
        apiBase: fallbackApiBase,
        token: "",
        projectId,
        selfLogProjectId: "",
        selfLogProjectKey: "",
      };
      if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
      cachedSettings = next;
      return next;
    }
    const parsed = JSON.parse(raw) as Partial<Settings>;
    const next = {
      apiBase:
        lockMode === "always"
          ? fallbackApiBase
          : normalizeApiBase(parsed.apiBase || fallbackApiBase),
      token: parsed.token || "",
      projectId: parsed.projectId || projectId,
      selfLogProjectId: parsed.selfLogProjectId || "",
      selfLogProjectKey: parsed.selfLogProjectKey || "",
    };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  } catch {
    const next = { apiBase: fallbackApiBase, token: "", projectId, selfLogProjectId: "", selfLogProjectKey: "" };
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
    let apiBase = next.apiBase;
    const lockMode = getApiBaseLockMode();
    const envApiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
    if (lockMode === "always") {
      apiBase = normalizeApiBase(envApiBase || "http://localhost:8080");
    } else if (lockMode === "once") {
      const raw = readStoredSettingsRaw();
      if (raw) {
        try {
          const parsed = JSON.parse(raw) as Partial<Settings>;
          if (parsed.apiBase) apiBase = parsed.apiBase;
        } catch {
        }
      }
    }
    const normalized = {
      ...next,
      apiBase: normalizeApiBase(apiBase),
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
  saveSettings({ apiBase: s.apiBase, token: "", projectId: "", selfLogProjectId: "", selfLogProjectKey: "" });
}

export function subscribeSettingsChange(listener: () => void): () => void {
  if (typeof window === "undefined") return () => {};
  const key = getSettingsStorageKey();

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
