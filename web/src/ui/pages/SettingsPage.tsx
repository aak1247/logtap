import { useEffect, useMemo, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import {
  cleanupEventsBefore,
  cleanupLogsBefore,
  createProjectKey,
  getCleanupPolicy,
  listProjectKeys,
  revokeProjectKey,
  runCleanupPolicy,
  upsertCleanupPolicy,
  type CleanupPolicy,
  type ProjectKey,
} from "../../lib/api";
import { clampFunnelDays, loadFunnelDays } from "../../lib/prefs";
import {
  canEditApiBase,
  clearAuth,
  loadSettings,
  normalizeApiBase,
  saveSettings,
} from "../../lib/storage";
import { Panel } from "../components/Panel";
import type { ApiSettings, EventDefinition, PropertyDefinition } from "../../lib/api";
import { listEventDefinitions, createEventDefinition, updateEventDefinition, listPropertyDefinitions, createPropertyDefinition, updatePropertyDefinition } from "../../lib/api";

export function SettingsPage() {
  const nav = useNavigate();
  const loc = useLocation();
  const funnelDays = useMemo(() => clampFunnelDays(loadFunnelDays()), []);
  const apiBaseEditable = canEditApiBase();

  const [settings, setSettings] = useState(() => loadSettings());
  const [apiBase, setApiBase] = useState(settings.apiBase);

  const [keys, setKeys] = useState<ProjectKey[]>([]);
  const [newKeyName, setNewKeyName] = useState("default");
  const [projectBusy, setProjectBusy] = useState(false);
  const [copiedKeyId, setCopiedKeyId] = useState<number | null>(null);
  const [projectErr, setProjectErr] = useState("");

  const [policy, setPolicy] = useState<CleanupPolicy | null>(null);
  const [policyDraft, setPolicyDraft] = useState<{
    enabled: boolean;
    logsDays: string;
    eventsDays: string;
    trackEventsDays: string;
    hourUTC: string;
    minuteUTC: string;
  } | null>(null);
  const [cleanupBusy, setCleanupBusy] = useState(false);
  const [cleanupMsg, setCleanupMsg] = useState("");
  const [manualBeforeLocal, setManualBeforeLocal] = useState(() =>
    toDateTimeLocal(new Date(Date.now() - 30 * 24 * 3600 * 1000)),
  );

  useEffect(() => {
    const next = loadSettings();
    setSettings(next);
    setApiBase(next.apiBase);
  }, [loc.key]);

  useEffect(() => {
    setKeys([]);
    setPolicy(null);
    setPolicyDraft(null);
    setCleanupMsg("");
    setProjectErr("");
  }, [settings.projectId]);

  useEffect(() => {
    if (loc.hash === "#cleanup") {
      window.setTimeout(() => {
        document.getElementById("cleanup")?.scrollIntoView({ block: "start" });
      }, 0);
    } else if (loc.hash === "#project") {
      window.setTimeout(() => {
        document.getElementById("project")?.scrollIntoView({ block: "start" });
      }, 0);
    }
  }, [loc.hash]);

  useEffect(() => {
    if (!settings.token) {
      nav("/login");
      return;
    }
    if (!settings.projectId) {
      nav("/projects");
    }
  }, [settings.token, settings.projectId, nav]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setProjectErr("");
        const projectId = settings.projectId.trim();
        if (!projectId) throw new Error("项目 ID 无效");
        const [res, cp] = await Promise.all([
          listProjectKeys(settings, projectId),
          getCleanupPolicy(settings).catch(() => null),
        ]);
        if (cancelled) return;
        setKeys(res.items);
        setPolicy(cp);
        if (!policyDraft) {
          setPolicyDraft({
            enabled: cp?.enabled ?? false,
            logsDays: String(cp?.logs_retention_days ?? 30),
            eventsDays: String(cp?.events_retention_days ?? 30),
            trackEventsDays: String(cp?.track_events_retention_days ?? 0),
            hourUTC: String(cp?.schedule_hour_utc ?? 3),
            minuteUTC: String(cp?.schedule_minute_utc ?? 0),
          });
        }
      } catch (e) {
        if (!cancelled) setProjectErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.token, settings.projectId]);

  const ingestURL = settings.projectId
    ? `${settings.apiBase.replace(/\/+$/, "")}/api/${settings.projectId}`
    : "";
  const firstKey = keys.find((k) => !k.revoked_at)?.key ?? "";

  const copyText = async (text: string): Promise<boolean> => {
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        return true;
      }
    } catch {
    }
    try {
      const el = document.createElement("textarea");
      el.value = text;
      el.setAttribute("readonly", "true");
      el.style.position = "fixed";
      el.style.top = "-9999px";
      el.style.left = "-9999px";
      document.body.appendChild(el);
      el.select();
      const ok = document.execCommand("copy");
      document.body.removeChild(el);
      return ok;
    } catch {
      return false;
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between">
        <div>
          <div className="text-lg font-semibold">设置</div>
          <div className="mt-1 text-sm text-zinc-400">
            API：{settings.apiBase} / 项目：{settings.projectId || "—"}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Link
            to="/projects"
            className="btn btn-md btn-outline"
          >
            切换项目
          </Link>
          <button
            className="btn btn-md btn-outline"
            onClick={() => {
              clearAuth();
              window.location.href = "/login";
            }}
          >
            退出登录
          </button>
        </div>
      </div>

      {apiBaseEditable ? (
        <Panel title="连接设置">
          <label className="block text-xs text-zinc-400">API Base（不要包含 /api）</label>
          <input
            value={apiBase}
            onChange={(e) => setApiBase(e.target.value)}
            placeholder="http://localhost:8080"
            className="mt-2 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          />
          <div className="mt-2 text-xs text-zinc-500">
            将会自动规范化：<span className="font-mono">{normalizeApiBase(apiBase)}</span>
          </div>

          <div className="mt-4 flex items-center justify-between">
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
              className="btn btn-md btn-primary"
              onClick={() => {
                const s = loadSettings();
                saveSettings({
                  apiBase: apiBase.trim(),
                  token: s.token,
                  projectId: s.projectId,
                  selfLogProjectId: s.selfLogProjectId,
                  selfLogProjectKey: s.selfLogProjectKey,
                });
                window.location.reload();
              }}
            >
              保存
            </button>
          </div>
        </Panel>
      ) : null}

      <div id="project" className="scroll-mt-24" />
      <Panel title="项目设置">
        {projectErr ? (
          <div className="mb-3 rounded-md border border-red-900/60 bg-red-950/40 p-3 text-xs text-red-200">
            {projectErr}
          </div>
        ) : null}

        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div className="rounded-lg border border-zinc-900 p-3">
            <div className="text-xs text-zinc-500">Project ID</div>
            <div className="mt-1 font-mono text-xs text-zinc-100">{settings.projectId}</div>
            <div className="mt-2 text-xs text-zinc-500">API Base</div>
            <div className="mt-1 font-mono text-xs text-zinc-300">{settings.apiBase}</div>
          </div>

          {settings.selfLogProjectId ? (
            <div className="rounded-lg border border-zinc-900 p-3">
              <div className="text-xs text-zinc-500">System Project（用于控制台/服务自上报）</div>
              <div className="mt-1 font-mono text-xs text-zinc-100">{settings.selfLogProjectId}</div>
              <div className="mt-3 flex gap-2">
                <button
                  className="btn btn-sm btn-outline"
                  onClick={() => {
                    saveSettings({ ...settings, projectId: settings.selfLogProjectId });
                    nav("/logs");
                  }}
                >
                  切换到系统项目日志
                </button>
              </div>
            </div>
          ) : null}

          <div className="rounded-lg border border-zinc-900 p-3">
            <div className="text-sm font-semibold">上报示例（用任一 active Key）</div>
            <div className="mt-3 space-y-2">
              <CodeBlock
                title="自定义日志（推荐）"
                text={`curl -sS -X POST "${ingestURL}/logs/" \\\n  -H "Content-Type: application/json" \\\n  -H "X-Project-Key: ${firstKey || "pk_xxx"}" \\\n  -d '{"level":"info","message":"signup","user":{"id":"u1"},"fields":{"k":"v"}}'`}
              />
              <CodeBlock
                title="Sentry DSN（可用于 SDK）"
                text={`DSN: ${formatDSN(settings.apiBase, settings.projectId, firstKey || "pk_xxx")}\nPOST: ${ingestURL}/envelope/`}
              />
            </div>
          </div>
        </div>

        <div className="mt-4 rounded-lg border border-zinc-900 p-3">
          <div className="flex items-center justify-between">
            <div className="text-sm font-semibold">上报鉴权 Key</div>
            <button
              className="btn btn-sm btn-outline"
              disabled={projectBusy}
              onClick={async () => {
                try {
                  if (!settings.projectId) return;
                  const projectId = settings.projectId.trim();
                  if (!projectId) throw new Error("项目 ID 无效");
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

          <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-2">
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
                      <div className="flex justify-end gap-2">
                        <button
                          className="btn btn-xs btn-outline"
                          disabled={projectBusy}
                          onClick={async () => {
                            const ok = await copyText(k.key);
                            if (!ok) return;
                            setCopiedKeyId(k.id);
                            window.setTimeout(() => {
                              setCopiedKeyId((prev) => (prev === k.id ? null : prev));
                            }, 1200);
                          }}
                        >
                          {copiedKeyId === k.id ? "已复制" : "复制"}
                        </button>
                        {!k.revoked_at ? (
                          <button
                            className="btn btn-xs btn-outline"
                            disabled={projectBusy}
                            onClick={async () => {
                              try {
                                if (!settings.projectId) return;
                                const projectId = settings.projectId.trim();
                                if (!projectId) throw new Error("项目 ID 无效");
                                setProjectErr("");
                                setProjectBusy(true);
                                await revokeProjectKey(settings, projectId, k.id);
                                const res = await listProjectKeys(settings, projectId);
                                setKeys(res.items);
                              } catch (e) {
                                setProjectErr(e instanceof Error ? e.message : String(e));
                              } finally {
                                setProjectBusy(false);
                              }
                            }}
                          >
                            吊销
                          </button>
                        ) : null}
                      </div>
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
      </Panel>

      <div id="cleanup" className="scroll-mt-24" />
      <Panel title="清理设置">
        {cleanupMsg ? (
          <div className="mb-3 rounded-lg border border-zinc-900 bg-zinc-950/60 p-3 text-sm text-zinc-200">
            {cleanupMsg}
          </div>
        ) : null}

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div className="rounded-xl border border-zinc-900 bg-zinc-950 p-4">
            <div className="text-sm font-semibold">自动清理</div>
            <div className="mt-1 text-xs text-zinc-500">
              通过保留天数定期删除更早的数据（UTC 定时）。
            </div>

            {!policyDraft ? (
              <div className="mt-3 text-sm text-zinc-500">未加载清理策略</div>
            ) : (
              <div className="mt-4 space-y-3">
                <label className="flex items-center justify-between gap-3 text-sm">
                  <span className="text-zinc-300">启用</span>
                  <input
                    type="checkbox"
                    checked={policyDraft.enabled}
                    onChange={(e) =>
                      setPolicyDraft((prev) =>
                        prev ? { ...prev, enabled: e.target.checked } : prev,
                      )
                    }
                  />
                </label>

                <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <Field
                    label="日志保留(天)"
                    value={policyDraft.logsDays}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, logsDays: v } : prev))
                    }
                    placeholder="30"
                  />
                  <Field
                    label="事件保留(天)"
                    value={policyDraft.eventsDays}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, eventsDays: v } : prev))
                    }
                    placeholder="30"
                  />
                  <Field
                    label="分析事件保留(天)"
                    value={policyDraft.trackEventsDays}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, trackEventsDays: v } : prev))
                    }
                    placeholder="0"
                  />
                  <Field
                    label="UTC 小时"
                    value={policyDraft.hourUTC}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, hourUTC: v } : prev))
                    }
                    placeholder="3"
                  />
                  <Field
                    label="UTC 分钟"
                    value={policyDraft.minuteUTC}
                    onChange={(v) =>
                      setPolicyDraft((prev) => (prev ? { ...prev, minuteUTC: v } : prev))
                    }
                    placeholder="0"
                  />
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <button
                    className="btn btn-md btn-primary"
                    disabled={cleanupBusy}
                    onClick={async () => {
                      if (!policyDraft) return;
                      try {
                        setCleanupBusy(true);
                        setCleanupMsg("");
                        const saved = await upsertCleanupPolicy(settings, {
                          enabled: policyDraft.enabled,
                          logs_retention_days: Number(policyDraft.logsDays || "0"),
                          events_retention_days: Number(policyDraft.eventsDays || "0"),
                          track_events_retention_days: Number(policyDraft.trackEventsDays || "0"),
                          schedule_hour_utc: Number(policyDraft.hourUTC || "0"),
                          schedule_minute_utc: Number(policyDraft.minuteUTC || "0"),
                        });
                        setPolicy(saved);
                        const trackDays = saved.track_events_retention_days ?? 0;
                        const warn =
                          trackDays > 0 && trackDays < funnelDays
                            ? `；注意：当前漏斗时间范围=${funnelDays} 天，但分析事件保留=${trackDays} 天。建议把“分析事件保留(天)”调到 ≥ ${funnelDays}，或把漏斗时间范围调到 ≤ ${trackDays}。`
                            : "";
                        setCleanupMsg(
                          `已保存：enabled=${saved.enabled} logs=${saved.logs_retention_days}d events=${saved.events_retention_days}d track=${trackDays}d next=${saved.next_run_at ?? "-"}${warn}`,
                        );
                      } catch (e) {
                        setCleanupMsg(e instanceof Error ? e.message : String(e));
                      } finally {
                        setCleanupBusy(false);
                      }
                    }}
                  >
                    保存策略
                  </button>
                  <button
                    className="btn btn-md btn-outline"
                    disabled={cleanupBusy || !policy?.enabled}
                    onClick={async () => {
                      try {
                        setCleanupBusy(true);
                        setCleanupMsg("");
                        const res = await runCleanupPolicy(settings);
                        setCleanupMsg(
                          `已清理：logs=${res.logs_deleted} (before ${res.logs_before || "-"}) events=${res.events_deleted} (before ${res.events_before || "-"}) track=${res.track_events_deleted} (before ${res.track_events_before || "-"})`,
                        );
                        setPolicy(await getCleanupPolicy(settings).catch(() => policy));
                      } catch (e) {
                        setCleanupMsg(e instanceof Error ? e.message : String(e));
                      } finally {
                        setCleanupBusy(false);
                      }
                    }}
                  >
                    按策略立即清理
                  </button>
                </div>

                {policy ? (
                  <div className="text-xs text-zinc-500">
                    last: {policy.last_run_at ? new Date(policy.last_run_at).toLocaleString() : "-"}
                    {" / "}
                    next: {policy.next_run_at ? new Date(policy.next_run_at).toLocaleString() : "-"}
                  </div>
                ) : null}
              </div>
            )}
          </div>

          <div className="rounded-xl border border-zinc-900 bg-zinc-950 p-4">
            <div className="text-sm font-semibold">手动清理</div>
            <div className="mt-1 text-xs text-zinc-500">
              手动指定“早于某时间”的数据清理（会删除对应日志/事件）。
            </div>

            <div className="mt-4 space-y-3">
              <div>
                <div className="text-xs text-zinc-400">before（本地时间）</div>
                <input
                  type="datetime-local"
                  value={manualBeforeLocal}
                  onChange={(e) => setManualBeforeLocal(e.target.value)}
                  className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
                />
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <button
                  className="btn btn-md btn-outline"
                  disabled={cleanupBusy}
                  onClick={async () => {
                    try {
                      setCleanupBusy(true);
                      setCleanupMsg("");
                      const before = fromDateTimeLocalToRFC3339(manualBeforeLocal);
                      const res = await cleanupLogsBefore(settings, before);
                      setCleanupMsg(`已清理日志：deleted=${res.deleted} (before ${before})`);
                    } catch (e) {
                      setCleanupMsg(e instanceof Error ? e.message : String(e));
                    } finally {
                      setCleanupBusy(false);
                    }
                  }}
                >
                  清理日志
                </button>
                <button
                  className="btn btn-md btn-outline"
                  disabled={cleanupBusy}
                  onClick={async () => {
                    try {
                      setCleanupBusy(true);
                      setCleanupMsg("");
                      const before = fromDateTimeLocalToRFC3339(manualBeforeLocal);
                      const res = await cleanupEventsBefore(settings, before);
                      setCleanupMsg(`已清理事件：deleted=${res.deleted} (before ${before})`);
                    } catch (e) {
                      setCleanupMsg(e instanceof Error ? e.message : String(e));
                    } finally {
                      setCleanupBusy(false);
                    }
                  }}
                >
                  清理事件
                </button>
              </div>
            </div>
          </div>
        </div>
      </Panel>

      <Panel title="事件定义（行为管理）">
        <EventSchemaPanel settings={settings} />
      </Panel>

      <Panel title="属性定义（事件属性／用户属性）">
        <PropertySchemaPanel settings={settings} />
      </Panel>
    </div>
  );
}


function EventSchemaPanel(props: { settings: ApiSettings }) {
  const { settings } = props;
  const [items, setItems] = useState<EventDefinition[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [name, setName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [category, setCategory] = useState("");
  const [description, setDescription] = useState("");
  const [owner, setOwner] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setErr("");
        const res = await listEventDefinitions(settings, { status: "active" });
        if (!cancelled) setItems(res.items ?? []);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId, settings.token]);

  const handleCreate = async () => {
    if (!settings.token || !settings.projectId) return;
    const n = name.trim();
    if (!n) {
      setErr("事件 name 不能为空");
      return;
    }
    setBusy(true);
    setErr("");
    try {
      const row = await createEventDefinition(settings, {
        name: n,
        display_name: displayName.trim() || undefined,
        category: category.trim() || undefined,
        description: description.trim() || undefined,
        owner: owner.trim() || undefined,
      });
      setItems((prev) => (prev ? [...prev, row] : [row]));
      setName("");
      setDisplayName("");
      setCategory("");
      setDescription("");
      setOwner("");
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {err ? (
        <div className="rounded-md border border-red-900/60 bg-red-950/40 p-2 text-xs text-red-200">
          {err}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
        <Field label="name（事件标识，英文）" value={name} onChange={setName} placeholder="signup" />
        <Field label="显示名" value={displayName} onChange={setDisplayName} placeholder="用户注册" />
        <Field label="分类（可选）" value={category} onChange={setCategory} placeholder="auth" />
        <Field label="Owner（可选）" value={owner} onChange={setOwner} placeholder="产品/负责人" />
      </div>
      <div>
        <div className="text-xs text-zinc-400">描述（可选）</div>
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="补充事件语义、触发条件等说明"
          className="mt-1 w-full resize-y rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          rows={2}
        />
      </div>
      <div className="flex justify-end">
        <button
          type="button"
          className="btn btn-sm btn-primary"
          onClick={handleCreate}
          disabled={busy}
        >
          {busy ? "保存中..." : "新增事件定义"}
        </button>
      </div>

      <div className="mt-2 overflow-x-auto text-sm">
        {items === null ? (
          <div className="text-zinc-500">加载中...</div>
        ) : items.length === 0 ? (
          <div className="text-zinc-500">暂无事件定义，可以先根据常见埋点补充。</div>
        ) : (
          <table className="w-full text-left text-xs">
            <thead className="text-zinc-500">
              <tr>
                <th className="py-2 pr-4">name</th>
                <th className="py-2 pr-4">显示名</th>
                <th className="py-2 pr-4">分类</th>
                <th className="py-2 pr-4">Owner</th>
                <th className="py-2 pr-4">状态</th>
                <th className="py-2 pr-4">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {items.map((it) => (
                <tr key={it.id} className="hover:bg-zinc-900/40">
                  <td className="py-2 pr-4 font-mono text-xs text-zinc-300">{it.name}</td>
                  <td className="py-2 pr-4 text-zinc-100">{it.display_name || it.name}</td>
                  <td className="py-2 pr-4 text-xs text-zinc-400">{it.category}</td>
                  <td className="py-2 pr-4 text-xs text-zinc-400">{it.owner}</td>
                  <td className="py-2 pr-4 text-xs text-zinc-400">{it.status}</td>
                  <td className="py-2 pr-4 text-right">
                    <button
                      type="button"
                      className="btn btn-xs btn-outline"
                      disabled={busy}
                      onClick={async () => {
                        try {
                          const nextStatus = it.status === "active" ? "inactive" : "active";
                          setBusy(true);
                          setErr("");
                          const updated = await updateEventDefinition(settings, it.name, {
                            status: nextStatus,
                          });
                          setItems((prev) =>
                            prev ? prev.map((row) => (row.id === updated.id ? updated : row)) : [updated],
                          );
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : String(e));
                        } finally {
                          setBusy(false);
                        }
                      }}
                    >
                      {it.status === "active" ? "停用" : "启用"}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function PropertySchemaPanel(props: { settings: ApiSettings }) {
  const { settings } = props;
  const [items, setItems] = useState<PropertyDefinition[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const [keyName, setKeyName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [type, setType] = useState<"string" | "enum" | "number">("string");
  const [description, setDescription] = useState("");
  const [enumValues, setEnumValues] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (!settings.token || !settings.projectId) return;
        setErr("");
        const res = await listPropertyDefinitions(settings, { status: "active" });
        if (!cancelled) setItems(res.items ?? []);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.projectId, settings.token]);

  const handleCreate = async () => {
    if (!settings.token || !settings.projectId) return;
    const k = keyName.trim();
    if (!k) {
      setErr("属性 key 不能为空");
      return;
    }
    if (type === "enum" && !enumValues.trim()) {
      setErr("枚举属性需要至少一个枚举值");
      return;
    }
    setBusy(true);
    setErr("");
    try {
      const enums = enumValues
        .split(/[\s,]+/)
        .map((s) => s.trim())
        .filter(Boolean);
      const row = await createPropertyDefinition(settings, {
        key: k,
        display_name: displayName.trim() || undefined,
        type,
        description: description.trim() || undefined,
        enum_values: enums.length > 0 ? enums : undefined,
      });
      setItems((prev) => (prev ? [...prev, row] : [row]));
      setKeyName("");
      setDisplayName("");
      setDescription("");
      setEnumValues("");
      setType("string");
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {err ? (
        <div className="rounded-md border border-red-900/60 bg-red-950/40 p-2 text-xs text-red-200">
          {err}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
        <Field label="key（属性标识，英文）" value={keyName} onChange={setKeyName} placeholder="plan" />
        <Field label="显示名" value={displayName} onChange={setDisplayName} placeholder="套餐" />
        <div>
          <div className="text-xs text-zinc-400">类型</div>
          <select
            value={type}
            onChange={(e) => setType(e.target.value as "string" | "enum" | "number")}
            className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          >
            <option value="string">string</option>
            <option value="enum">enum</option>
            <option value="number">number</option>
          </select>
        </div>
        {type === "enum" ? (
          <div>
            <div className="text-xs text-zinc-400">枚举值（逗号或空格分隔）</div>
            <input
              value={enumValues}
              onChange={(e) => setEnumValues(e.target.value)}
              placeholder="free pro enterprise"
              className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
            />
          </div>
        ) : (
          <div className="text-xs text-zinc-500">&nbsp;</div>
        )}
      </div>
      <div>
        <div className="text-xs text-zinc-400">描述（可选）</div>
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="说明该属性的含义、取值范围等"
          className="mt-1 w-full resize-y rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
          rows={2}
        />
      </div>
      <div className="flex justify-end">
        <button
          type="button"
          className="btn btn-sm btn-primary"
          onClick={handleCreate}
          disabled={busy}
        >
          {busy ? "保存中..." : "新增属性定义"}
        </button>
      </div>

      <div className="mt-2 overflow-x-auto text-sm">
        {items === null ? (
          <div className="text-zinc-500">加载中...</div>
        ) : items.length === 0 ? (
          <div className="text-zinc-500">暂无属性定义，可以先根据埋点字段补充。</div>
        ) : (
          <table className="w-full text-left text-xs">
            <thead className="text-zinc-500">
              <tr>
                <th className="py-2 pr-4">key</th>
                <th className="py-2 pr-4">显示名</th>
                <th className="py-2 pr-4">类型</th>
                <th className="py-2 pr-4">状态</th>
                <th className="py-2 pr-4">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {items.map((it) => (
                <tr key={it.id} className="hover:bg-zinc-900/40">
                  <td className="py-2 pr-4 font-mono text-xs text-zinc-300">{it.key}</td>
                  <td className="py-2 pr-4 text-zinc-100">{it.display_name || it.key}</td>
                  <td className="py-2 pr-4 text-xs text-zinc-400">{it.type}</td>
                  <td className="py-2 pr-4 text-xs text-zinc-400">{it.status}</td>
                  <td className="py-2 pr-4 text-right">
                    <button
                      type="button"
                      className="btn btn-xs btn-outline"
                      disabled={busy}
                      onClick={async () => {
                        try {
                          const nextStatus = it.status === "active" ? "inactive" : "active";
                          setBusy(true);
                          setErr("");
                          const updated = await updatePropertyDefinition(settings, it.key, {
                            status: nextStatus,
                          });
                          setItems((prev) =>
                            prev ? prev.map((row) => (row.id === updated.id ? updated : row)) : [updated],
                          );
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : String(e));
                        } finally {
                          setBusy(false);
                        }
                      }}
                    >
                      {it.status === "active" ? "停用" : "启用"}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}


function CodeBlock(props: { title: string; text: string }) {
  return (
    <div className="rounded-lg border border-zinc-900 bg-zinc-950 p-3">
      <div className="flex items-center justify-between">
        <div className="text-xs text-zinc-500">{props.title}</div>
        <button
          className="btn btn-xs btn-outline"
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

function formatDSN(apiBase: string, projectId: string, key: string) {
  try {
    const u = new URL(apiBase);
    return `${u.protocol}//${encodeURIComponent(key)}@${u.host}/${projectId}`;
  } catch {
    return `http://${key}@localhost:8080/${projectId}`;
  }
}

function toDateTimeLocal(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const yyyy = d.getFullYear();
  const mm = pad(d.getMonth() + 1);
  const dd = pad(d.getDate());
  const hh = pad(d.getHours());
  const mi = pad(d.getMinutes());
  return `${yyyy}-${mm}-${dd}T${hh}:${mi}`;
}

function fromDateTimeLocalToRFC3339(localValue: string): string {
  const raw = (localValue || "").trim();
  if (!raw) throw new Error("before required");
  const d = new Date(raw);
  if (!Number.isFinite(d.getTime())) throw new Error("invalid before");
  return d.toISOString();
}

function Field(props: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  return (
    <div>
      <div className="text-xs text-zinc-400">{props.label}</div>
      <input
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        placeholder={props.placeholder}
        className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
      />
    </div>
  );
}
