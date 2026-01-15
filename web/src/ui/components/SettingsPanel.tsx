import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { createProjectKey, listProjectKeys, revokeProjectKey, type ProjectKey } from "../../lib/api";
import { clearAuth, loadSettings, saveSettings } from "../../lib/storage";

export function SettingsPanel() {
  const [searchParams, setSearchParams] = useSearchParams();
  const openFromQuery = searchParams.get("settings") === "project";
  const searchKey = searchParams.toString();
  const [open, setOpen] = useState(false);
  const [settings, setSettings] = useState(() => loadSettings());
  const [apiBase, setApiBase] = useState(settings.apiBase);
  const [keys, setKeys] = useState<ProjectKey[]>([]);
  const [newKeyName, setNewKeyName] = useState("default");
  const [projectBusy, setProjectBusy] = useState(false);
  const [projectErr, setProjectErr] = useState("");

  useEffect(() => {
    if (openFromQuery) setOpen(true);
  }, [openFromQuery]);

  useEffect(() => {
    if (!open && !openFromQuery) return;
    const next = loadSettings();
    setSettings(next);
    setApiBase(next.apiBase);
  }, [open, openFromQuery, searchKey]);

  useEffect(() => {
    if (!open || !settings.token || !settings.projectId) {
      setKeys([]);
      setProjectErr("");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        setProjectErr("");
        const projectId = Number(settings.projectId);
        if (!Number.isFinite(projectId) || projectId <= 0) {
          throw new Error("项目 ID 无效");
        }
        const res = await listProjectKeys(settings, projectId);
        if (!cancelled) setKeys(res.items);
      } catch (e) {
        if (!cancelled) setProjectErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, settings.apiBase, settings.token, settings.projectId]);

  const closePanel = () => {
    setOpen(false);
    if (openFromQuery) {
      const next = new URLSearchParams(searchParams);
      next.delete("settings");
      next.delete("projectId");
      setSearchParams(next, { replace: true });
    }
  };

  const togglePanel = () => {
    if (open || openFromQuery) {
      closePanel();
      return;
    }
    setOpen(true);
  };

  const ingestURL = settings.projectId
    ? `${settings.apiBase.replace(/\/+$/, "")}/api/${settings.projectId}`
    : "";
  const firstKey = keys.find((k) => !k.revoked_at)?.key ?? "";

  return (
    <div className="relative">
      <button
        className="rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-900"
        onClick={togglePanel}
      >
        设置
      </button>
      {open ? (
        <div className="absolute right-0 mt-2 w-[26rem] max-h-[80vh] overflow-y-auto rounded-lg border border-zinc-800 bg-zinc-950 p-4 shadow-xl">
          <div className="mb-3 text-sm font-semibold">连接设置</div>
          <label className="block text-xs text-zinc-400">API Base</label>
          <input
            value={apiBase}
            onChange={(e) => {
              const raw = e.target.value;
              setApiBase(raw);
              const base = raw.trim().replace(/\/+$/, "");
              if (settings.apiBase === base) return;
              const next = { ...settings, apiBase: base };
              setSettings(next);
              saveSettings(next);
            }}
            placeholder="http://localhost:8080"
            className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          />
          {settings.token ? (
            <div className="mt-4 border-t border-zinc-900 pt-4">
              <div className="text-sm font-semibold">项目详情</div>
              {!settings.projectId ? (
                <div className="mt-2 text-xs text-zinc-500">未选择项目，请先切换项目。</div>
              ) : (
                <div className="mt-3 space-y-4">
                  {projectErr ? (
                    <div className="rounded-md border border-red-900/60 bg-red-950/40 p-3 text-xs text-red-200">
                      {projectErr}
                    </div>
                  ) : null}
                  <div className="rounded-lg border border-zinc-900 p-3">
                    <div className="text-xs text-zinc-500">Project ID</div>
                    <div className="mt-1 font-mono text-xs text-zinc-100">
                      {settings.projectId}
                    </div>
                    <div className="mt-2 text-xs text-zinc-500">API Base</div>
                    <div className="mt-1 font-mono text-xs text-zinc-300">
                      {settings.apiBase}
                    </div>
                  </div>

                  <div className="rounded-lg border border-zinc-900 p-3">
                    <div className="flex items-center justify-between">
                      <div className="text-sm font-semibold">上报鉴权 Key</div>
                      <button
                        className="rounded-md border border-zinc-800 bg-zinc-950 px-3 py-1.5 text-xs text-zinc-200 hover:bg-zinc-900 disabled:opacity-60"
                        disabled={projectBusy}
                        onClick={async () => {
                          try {
                            if (!settings.projectId) return;
                            const projectId = Number(settings.projectId);
                            if (!Number.isFinite(projectId) || projectId <= 0) {
                              throw new Error("项目 ID 无效");
                            }
                            setProjectErr("");
                            setProjectBusy(true);
                            const k = await createProjectKey(settings, projectId, newKeyName.trim());
                            setNewKeyName("default");
                            setKeys((prev) => [...prev, k]);
                          } catch (e) {
                            setProjectErr(e instanceof Error ? e.message : String(e));
                          } finally {
                            setProjectBusy(false);
                          }
                        }}
                      >
                        新建 Key
                      </button>
                    </div>
                    <div className="mt-2 grid grid-cols-1 gap-2">
                      <div>
                        <div className="text-xs text-zinc-400">Key 名称</div>
                        <input
                          value={newKeyName}
                          onChange={(e) => setNewKeyName(e.target.value)}
                          placeholder="default"
                          className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                        />
                      </div>
                      <div>
                        <div className="text-xs text-zinc-400">当前可用 Key（示例）</div>
                        <div className="mt-1 truncate rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-200">
                          {firstKey || "(none)"}
                        </div>
                      </div>
                    </div>

                    <div className="mt-3 overflow-x-auto">
                      <table className="w-full text-left text-xs">
                        <thead className="text-xs text-zinc-500">
                          <tr>
                            <th className="py-2 pr-4">Name</th>
                            <th className="py-2 pr-4">Key</th>
                            <th className="py-2 pr-4">Status</th>
                            <th className="py-2 pr-0" />
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-zinc-900">
                          {keys.map((k) => (
                            <tr key={k.id} className="hover:bg-zinc-900/40">
                              <td className="py-2 pr-4 text-zinc-100">{k.name}</td>
                              <td className="py-2 pr-4 font-mono text-xs text-zinc-300">
                                <span className="block max-w-[18rem] truncate" title={k.key}>
                                  {k.key}
                                </span>
                              </td>
                              <td className="py-2 pr-4 text-xs text-zinc-400">
                                {k.revoked_at ? "revoked" : "active"}
                              </td>
                              <td className="py-2 pr-0 text-right">
                                {!k.revoked_at ? (
                                  <button
                                    className="rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-200 hover:bg-zinc-900 disabled:opacity-60"
                                    disabled={projectBusy}
                                    onClick={async () => {
                                      try {
                                        if (!settings.projectId) return;
                                        const projectId = Number(settings.projectId);
                                        if (!Number.isFinite(projectId) || projectId <= 0) {
                                          throw new Error("项目 ID 无效");
                                        }
                                        setProjectErr("");
                                        setProjectBusy(true);
                                        await revokeProjectKey(settings, projectId, k.id);
                                        const res = await listProjectKeys(settings, projectId);
                                        setKeys(res.items);
                                      } catch (e) {
                                        setProjectErr(
                                          e instanceof Error ? e.message : String(e),
                                        );
                                      } finally {
                                        setProjectBusy(false);
                                      }
                                    }}
                                  >
                                    吊销
                                  </button>
                                ) : null}
                              </td>
                            </tr>
                          ))}
                          {keys.length === 0 ? (
                            <tr>
                              <td className="py-6 text-sm text-zinc-500" colSpan={4}>
                                暂无 Key
                              </td>
                            </tr>
                          ) : null}
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <div className="rounded-lg border border-zinc-900 p-3">
                    <div className="text-sm font-semibold">上报示例（用任一 active Key）</div>
                    <div className="mt-3 space-y-2">
                      <CodeBlock
                        title="自定义日志（推荐）"
                        text={`curl -sS -X POST "${ingestURL}/logs/" \\\n  -H "Content-Type: application/json" \\\n  -H "X-Project-Key: ${firstKey || "pk_xxx"}" \\\n  -d '{"level":"info","message":"signup","user":{"id":"u1"},"fields":{"k":"v"}}'`}
                      />
                      <CodeBlock
                        title="Sentry DSN（可用于 SDK）"
                        text={`DSN: ${formatDSN(settings.apiBase, Number(settings.projectId), firstKey || "pk_xxx")}\nPOST: ${ingestURL}/envelope/`}
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="mt-3 text-xs text-zinc-500">未登录</div>
          )}

          <div className="mt-4 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <button
                className="text-xs text-zinc-400 hover:text-zinc-200"
                onClick={() => {
                  const s = loadSettings();
                  setSettings(s);
                  setApiBase(s.apiBase);
                }}
              >
                重置
              </button>
              <button
                className="text-xs text-zinc-400 hover:text-zinc-200"
                onClick={() => {
                  clearAuth();
                  closePanel();
                  window.location.href = "/login";
                }}
              >
                退出登录
              </button>
            </div>
            <div className="flex gap-2">
              <button
                className="rounded-md px-3 py-2 text-sm text-zinc-300 hover:bg-zinc-900"
                onClick={closePanel}
              >
                关闭
              </button>
              <button
                className="rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500"
                onClick={() => {
                  const s = loadSettings();
                  saveSettings({
                    apiBase: apiBase.trim(),
                    token: s.token,
                    projectId: s.projectId,
                  });
                  closePanel();
                  window.location.reload();
                }}
              >
                保存
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function CodeBlock(props: { title: string; text: string }) {
  return (
    <div className="rounded-lg border border-zinc-900 bg-zinc-950 p-3">
      <div className="flex items-center justify-between">
        <div className="text-xs text-zinc-500">{props.title}</div>
        <button
          className="rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-200 hover:bg-zinc-900"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(props.text);
            } catch {
            }
          }}
        >
          Copy
        </button>
      </div>
      <pre className="mt-2 overflow-auto rounded bg-zinc-900/40 p-3 text-xs text-zinc-100">
        {props.text}
      </pre>
    </div>
  );
}

function formatDSN(apiBase: string, projectId: number, key: string) {
  try {
    const u = new URL(apiBase);
    return `${u.protocol}//${encodeURIComponent(key)}@${u.host}/${projectId}`;
  } catch {
    return `http://${key}@localhost:8080/${projectId}`;
  }
}
