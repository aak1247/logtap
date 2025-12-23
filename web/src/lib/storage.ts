type Settings = {
  apiBase: string;
  token: string;
  projectId: string;
};

const key = "logtap:settings:v1";

export function loadSettings(): Settings {
  const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
  const projectId =
    (import.meta.env.VITE_DEFAULT_PROJECT_ID as string | undefined) ?? "";

  try {
    const raw = localStorage.getItem(key);
    if (!raw)
      return {
        apiBase: apiBase || "http://localhost:8080",
        token: "",
        projectId,
      };
    const parsed = JSON.parse(raw) as Partial<Settings>;
    return {
      apiBase: parsed.apiBase || apiBase || "http://localhost:8080",
      token: parsed.token || "",
      projectId: parsed.projectId || projectId,
    };
  } catch {
    return { apiBase: apiBase || "http://localhost:8080", token: "", projectId };
  }
}

export function saveSettings(next: Settings) {
  localStorage.setItem(key, JSON.stringify(next));
}

export function clearAuth() {
  const s = loadSettings();
  saveSettings({ apiBase: s.apiBase, token: "", projectId: "" });
}
