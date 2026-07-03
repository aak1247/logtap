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

function configuredApiBase(): string {
  const envApiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
  return normalizeApiBase(envApiBase);
}

export function loadSettings(): Settings {
  const projectId =
    (import.meta.env.VITE_DEFAULT_PROJECT_ID as string | undefined) ?? "";
  const apiBase = configuredApiBase();

  if (typeof window === "undefined") {
    const next = { apiBase, token: "", projectId, selfLogProjectId: "", selfLogProjectKey: "" };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  }

  try {
    const raw = readStoredSettingsRaw();
    if (!raw) {
      const next = {
        apiBase,
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
      apiBase,
      token: parsed.token || "",
      projectId: parsed.projectId || projectId,
      selfLogProjectId: parsed.selfLogProjectId || "",
      selfLogProjectKey: parsed.selfLogProjectKey || "",
    };
    if (cachedSettings && sameSettings(cachedSettings, next)) return cachedSettings;
    cachedSettings = next;
    return next;
  } catch {
    const next = { apiBase, token: "", projectId, selfLogProjectId: "", selfLogProjectKey: "" };
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
      apiBase: configuredApiBase(),
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
