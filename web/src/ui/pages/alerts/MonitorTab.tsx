import { useEffect, useMemo, useState } from "react";
import {
  createMonitor,
  deleteMonitor,
  getDetectorSchema,
  listDetectors,
  listMonitorRuns,
  listMonitors,
  runMonitorNow,
  testMonitor,
  updateMonitor,
  type ApiSettings,
  type DetectorDescriptor,
  type MonitorDefinition,
  type MonitorRun,
  type MonitorTestResult,
} from "../../../lib/api";
import { Panel } from "../../components/Panel";
import { SchemaForm, type JsonSchema, type FieldError } from "../../components/schema-form";

type MonitorFormState = {
  monitorId: number;
  name: string;
  detectorType: string;
  intervalSec: string;
  timeoutMs: string;
  enabled: boolean;
  configJSON: string;
};

function createDefaultMonitorForm(): MonitorFormState {
  return {
    monitorId: 0,
    name: "",
    detectorType: "",
    intervalSec: "60",
    timeoutMs: "5000",
    enabled: true,
    configJSON: "{}",
  };
}

export function MonitorTab(props: { settings: ApiSettings }) {
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");

  const [detectors, setDetectors] = useState<DetectorDescriptor[]>([]);
  const [detectorSchemas, setDetectorSchemas] = useState<Record<string, unknown>>({});
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [monitors, setMonitors] = useState<MonitorDefinition[]>([]);

  const [form, setForm] = useState<MonitorFormState>(() => createDefaultMonitorForm());
  const [configMode, setConfigMode] = useState<"form" | "json">("form");
  const [formErrors, setFormErrors] = useState<FieldError[]>([]);

  const [selectedRunsMonitorId, setSelectedRunsMonitorId] = useState(0);
  const [runs, setRuns] = useState<MonitorRun[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);

  const [testResult, setTestResult] = useState<MonitorTestResult | null>(null);

  const sortedDetectors = useMemo(() => {
    return [...detectors].sort((a, b) => a.type.localeCompare(b.type));
  }, [detectors]);

  const currentSchema = form.detectorType ? detectorSchemas[form.detectorType] : undefined;

  // Parse configJSON to object for SchemaForm
  const configObject = useMemo(() => {
    try {
      return parseConfigToRecord(form.configJSON);
    } catch {
      return {};
    }
  }, [form.configJSON]);

  // Typed schema for SchemaForm
  const typedSchema = useMemo((): JsonSchema | null => {
    if (!currentSchema || typeof currentSchema !== "object") return null;
    const schema = currentSchema as Record<string, unknown>;
    if (schema.type !== "object" || typeof schema.properties !== "object") {
      return null;
    }
    return schema as unknown as JsonSchema;
  }, [currentSchema]);

  useEffect(() => {
    void loadBaseData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.settings.apiBase, props.settings.token, props.settings.projectId]);

  useEffect(() => {
    const detectorType = form.detectorType.trim().toLowerCase();
    if (!detectorType) return;
    if (detectorSchemas[detectorType] !== undefined) return;
    void loadDetectorSchema(detectorType);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [form.detectorType]);

  async function loadBaseData() {
    try {
      setLoading(true);
      setErr("");
      const [detectorsRes, monitorsRes] = await Promise.all([
        listDetectors(props.settings),
        listMonitors(props.settings),
      ]);
      setDetectors(detectorsRes.items || []);
      setMonitors(monitorsRes.items || []);
      if (!form.detectorType && detectorsRes.items.length > 0) {
        setForm((prev) => ({ ...prev, detectorType: detectorsRes.items[0].type }));
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  async function loadDetectorSchema(detectorType: string) {
    try {
      setSchemaLoading(true);
      const res = await getDetectorSchema(props.settings, detectorType);
      setDetectorSchemas((prev) => ({ ...prev, [detectorType]: res.schema ?? {} }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setDetectorSchemas((prev) => ({ ...prev, [detectorType]: {} }));
    } finally {
      setSchemaLoading(false);
    }
  }

  async function loadRuns(monitorId: number) {
    try {
      setRunsLoading(true);
      setErr("");
      const res = await listMonitorRuns(props.settings, monitorId, { limit: 50 });
      setSelectedRunsMonitorId(monitorId);
      setRuns(res.items || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setRunsLoading(false);
    }
  }

  async function runAction(successMessage: string, fn: () => Promise<void>, reload = true) {
    try {
      setBusy(successMessage);
      setErr("");
      setMsg("");
      await fn();
      setMsg(successMessage);
      if (reload) {
        const monitorsRes = await listMonitors(props.settings);
        setMonitors(monitorsRes.items || []);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  }

  function startCreate() {
    setForm((prev) => ({
      ...createDefaultMonitorForm(),
      detectorType: prev.detectorType || sortedDetectors[0]?.type || "",
    }));
    setFormErrors([]);
    setTestResult(null);
  }

  function startEdit(monitor: MonitorDefinition) {
    setForm({
      monitorId: monitor.id,
      name: monitor.name || "",
      detectorType: (monitor.detector_type || "").trim().toLowerCase(),
      intervalSec: String(monitor.interval_sec || 60),
      timeoutMs: String(monitor.timeout_ms || 5000),
      enabled: Boolean(monitor.enabled),
      configJSON: stringifyJSON(monitor.config),
    });
    setFormErrors([]);
    setTestResult(null);
  }

  function handleConfigChange(newConfig: Record<string, unknown>) {
    setForm((prev) => ({
      ...prev,
      configJSON: stringifyJSON(newConfig),
    }));
    // Clear form errors when config changes
    setFormErrors([]);
  }

  async function saveMonitor() {
    const name = form.name.trim();
    if (!name) throw new Error("监控名称不能为空");
    const detectorType = form.detectorType.trim().toLowerCase();
    if (!detectorType) throw new Error("detectorType 不能为空");
    const config = parseConfigObject(form.configJSON);
    const intervalSec = toPositiveInt(form.intervalSec, 60);
    const timeoutMs = toPositiveInt(form.timeoutMs, 5000);

    if (form.monitorId > 0) {
      await updateMonitor(props.settings, form.monitorId, {
        name,
        detectorType,
        config,
        intervalSec,
        timeoutMs,
        enabled: form.enabled,
      });
      return;
    }

    await createMonitor(props.settings, {
      name,
      detectorType,
      config,
      intervalSec,
      timeoutMs,
      enabled: form.enabled,
    });
  }

  const selectedRunsMonitor = monitors.find((item) => item.id === selectedRunsMonitorId);
  const testedMonitor = testResult ? monitors.find((item) => item.id === testResult.monitorId) : undefined;

  return (
    <div className="space-y-4">
      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}
      {msg ? (
        <div className="rounded-xl border border-emerald-900/50 bg-emerald-950/30 p-4 text-sm text-emerald-200">
          {msg}
        </div>
      ) : null}

      <Panel
        title="监控插件说明"
        right={
          <button
            className="btn btn-md btn-outline"
            disabled={loading || Boolean(busy)}
            onClick={() => void loadBaseData()}
          >
            {loading ? "刷新中..." : "刷新"}
          </button>
        }
      >
        <div className="text-sm text-zinc-300">
          <div>- `Run`：异步入队调度，按 worker 流程执行并可能触发通知。</div>
          <div>- `Test`：同步预检 detector，不触发通知链路，只返回信号摘要。</div>
        </div>
      </Panel>

      <Panel
        title={form.monitorId > 0 ? `编辑监控 #${form.monitorId}` : "新建监控"}
        right={
          <button className="btn btn-md btn-outline" disabled={Boolean(busy)} onClick={startCreate}>
            清空
          </button>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-5">
            <InputField label="名称" value={form.name} onChange={(v) => setForm((prev) => ({ ...prev, name: v }))} placeholder="api-health-check" />
            <SelectField
              label="Detector"
              value={form.detectorType}
              onChange={(v) => setForm((prev) => ({ ...prev, detectorType: v }))}
              options={sortedDetectors.map((d) => ({ value: d.type, label: d.type }))}
            />
            <InputField
              label="intervalSec"
              value={form.intervalSec}
              onChange={(v) => setForm((prev) => ({ ...prev, intervalSec: v }))}
              placeholder="60"
            />
            <InputField
              label="timeoutMs"
              value={form.timeoutMs}
              onChange={(v) => setForm((prev) => ({ ...prev, timeoutMs: v }))}
              placeholder="5000"
            />
            <ToggleField
              label="启用"
              checked={form.enabled}
              onChange={(v) => setForm((prev) => ({ ...prev, enabled: v }))}
            />
          </div>

          {/* Config Section with Mode Toggle */}
          <div className="rounded-lg border border-zinc-900 bg-zinc-950/40 p-4">
            <div className="mb-3 flex items-center justify-between">
              <div className="text-xs text-zinc-400">
                插件配置
                {schemaLoading ? "（加载中）" : ""}
              </div>
              <div className="flex gap-1 rounded-md border border-zinc-800 bg-zinc-950 p-0.5">
                <button
                  type="button"
                  onClick={() => setConfigMode("form")}
                  className={`rounded px-2 py-1 text-xs ${
                    configMode === "form"
                      ? "bg-indigo-600 text-white"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  表单模式
                </button>
                <button
                  type="button"
                  onClick={() => setConfigMode("json")}
                  className={`rounded px-2 py-1 text-xs ${
                    configMode === "json"
                      ? "bg-indigo-600 text-white"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  JSON模式
                </button>
              </div>
            </div>

            {configMode === "form" ? (
              typedSchema ? (
                <SchemaForm
                  schema={typedSchema}
                  value={configObject}
                  onChange={handleConfigChange}
                  errors={formErrors}
                  disabled={Boolean(busy)}
                />
              ) : (
                <div className="py-4 text-center text-sm text-zinc-500">
                  {schemaLoading ? "正在加载表单配置..." : "请先选择检测器类型"}
                </div>
              )
            ) : (
              <JsonField
                label=""
                value={form.configJSON}
                onChange={(v) => {
                  setForm((prev) => ({ ...prev, configJSON: v }));
                  setFormErrors([]);
                }}
                rows={10}
              />
            )}
          </div>

          {/* Schema Reference (collapsible) */}
          <details className="rounded-lg border border-zinc-900 bg-zinc-950/40">
            <summary className="cursor-pointer px-3 py-2 text-xs text-zinc-400 hover:text-zinc-300">
              Detector Schema 参考
            </summary>
            <div className="border-t border-zinc-900 p-3">
              <pre className="max-h-64 overflow-auto rounded-md border border-zinc-900 bg-zinc-950 p-3 font-mono text-xs text-zinc-300">
                {stringifyJSON(currentSchema ?? {})}
              </pre>
            </div>
          </details>

          <div className="flex flex-wrap gap-2">
            <button
              className="btn btn-md btn-primary"
              disabled={Boolean(busy)}
              onClick={() =>
                void runAction(form.monitorId > 0 ? "监控已更新" : "监控已创建", async () => {
                  await saveMonitor();
                  if (form.monitorId === 0) startCreate();
                })
              }
            >
              {form.monitorId > 0 ? "保存修改" : "创建监控"}
            </button>
            {form.monitorId > 0 ? (
              <button
                className="btn btn-md btn-outline"
                disabled={Boolean(busy)}
                onClick={() => void runAction("配置已重置", async () => startCreate(), false)}
              >
                取消编辑
              </button>
            ) : null}
          </div>
        </div>
      </Panel>

      <Panel title={`Detector 列表（${sortedDetectors.length}）`}>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-zinc-500">
              <tr>
                <th className="py-2 pr-4">type</th>
                <th className="py-2 pr-4">mode</th>
                <th className="py-2 pr-0">path</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {sortedDetectors.map((d) => (
                <tr key={`${d.type}-${d.path || "builtin"}`}>
                  <td className="py-2 pr-4 font-mono text-zinc-300">{d.type}</td>
                  <td className="py-2 pr-4">{d.mode || "-"}</td>
                  <td className="py-2 pr-0 font-mono text-zinc-400">{d.path || "-"}</td>
                </tr>
              ))}
              {sortedDetectors.length === 0 ? (
                <tr>
                  <td className="py-6 text-sm text-zinc-500" colSpan={3}>
                    暂无 detector
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Panel>

      <Panel title={`监控项（${monitors.length}）`}>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-zinc-500">
              <tr>
                <th className="py-2 pr-3">ID</th>
                <th className="py-2 pr-3">名称</th>
                <th className="py-2 pr-3">detector</th>
                <th className="py-2 pr-3">间隔/超时</th>
                <th className="py-2 pr-3">状态</th>
                <th className="py-2 pr-3">nextRunAt</th>
                <th className="py-2 pr-0">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-900">
              {monitors.map((m) => (
                <tr key={m.id} className="hover:bg-zinc-900/40">
                  <td className="py-2 pr-3 font-mono text-zinc-400">#{m.id}</td>
                  <td className="py-2 pr-3 text-zinc-100">{m.name}</td>
                  <td className="py-2 pr-3 font-mono text-zinc-300">{m.detector_type}</td>
                  <td className="py-2 pr-3 text-zinc-300">{m.interval_sec}s / {m.timeout_ms}ms</td>
                  <td className="py-2 pr-3">
                    <StatusPill ok={m.enabled} okText="enabled" noText="disabled" />
                  </td>
                  <td className="py-2 pr-3 text-zinc-400">{toLocalTime(m.next_run_at)}</td>
                  <td className="py-2 pr-0">
                    <div className="flex flex-wrap gap-1">
                      <button className="btn btn-xs btn-outline" disabled={Boolean(busy)} onClick={() => startEdit(m)}>
                        编辑
                      </button>
                      <button
                        className="btn btn-xs btn-outline"
                        disabled={Boolean(busy)}
                        onClick={() =>
                          void runAction(m.enabled ? "监控已停用" : "监控已启用", async () => {
                            await updateMonitor(props.settings, m.id, { enabled: !m.enabled });
                          })
                        }
                      >
                        {m.enabled ? "停用" : "启用"}
                      </button>
                      <button
                        className="btn btn-xs btn-outline"
                        disabled={Boolean(busy)}
                        onClick={() =>
                          void runAction("已触发异步调度", async () => {
                            await runMonitorNow(props.settings, m.id);
                          })
                        }
                      >
                        Run
                      </button>
                      <button
                        className="btn btn-xs btn-outline"
                        disabled={Boolean(busy)}
                        onClick={() =>
                          void runAction("试运行完成", async () => {
                            const res = await testMonitor(props.settings, m.id);
                            setTestResult(res);
                          }, false)
                        }
                      >
                        Test
                      </button>
                      <button className="btn btn-xs btn-outline" disabled={Boolean(busy)} onClick={() => void loadRuns(m.id)}>
                        Runs
                      </button>
                      <button
                        className="btn btn-xs btn-outline"
                        disabled={Boolean(busy)}
                        onClick={() => {
                          if (!window.confirm(`确定删除监控 #${m.id} 吗？`)) return;
                          void runAction("监控已删除", async () => {
                            await deleteMonitor(props.settings, m.id);
                            if (selectedRunsMonitorId === m.id) {
                              setSelectedRunsMonitorId(0);
                              setRuns([]);
                            }
                            if (testResult?.monitorId === m.id) {
                              setTestResult(null);
                            }
                            if (form.monitorId === m.id) {
                              startCreate();
                            }
                          });
                        }}
                      >
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {monitors.length === 0 ? (
                <tr>
                  <td className="py-6 text-sm text-zinc-500" colSpan={7}>
                    暂无监控项
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Panel>

      {testResult ? (
        <Panel title={`试运行结果（#${testResult.monitorId} ${testedMonitor?.name || ""}）`}>
          <div className="space-y-2 text-sm text-zinc-300">
            <div>detector: <span className="font-mono">{testResult.detectorType}</span></div>
            <div>signalCount: <span className="font-mono">{testResult.signalCount}</span></div>
            <div>elapsedMs: <span className="font-mono">{testResult.elapsedMs}</span></div>
            <div className="text-xs text-zinc-400">该结果仅用于预检，不会触发通知投递。</div>
          </div>
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="text-zinc-500">
                <tr>
                  <th className="py-2 pr-3">source</th>
                  <th className="py-2 pr-3">severity</th>
                  <th className="py-2 pr-3">status</th>
                  <th className="py-2 pr-3">message</th>
                  <th className="py-2 pr-0">occurredAt</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-900">
                {testResult.samples.map((item, idx) => (
                  <tr key={`${item.source}-${idx}`}>
                    <td className="py-2 pr-3 font-mono text-zinc-300">{item.sourceType}/{item.source}</td>
                    <td className="py-2 pr-3">{item.severity}</td>
                    <td className="py-2 pr-3">{item.status}</td>
                    <td className="py-2 pr-3 text-zinc-300">{item.message || "-"}</td>
                    <td className="py-2 pr-0 text-zinc-400">{toLocalTime(item.occurredAt)}</td>
                  </tr>
                ))}
                {testResult.samples.length === 0 ? (
                  <tr>
                    <td className="py-5 text-zinc-500" colSpan={5}>
                      无 sample 信号
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </Panel>
      ) : null}

      {selectedRunsMonitorId > 0 ? (
        <Panel title={`运行历史（#${selectedRunsMonitorId} ${selectedRunsMonitor?.name || ""}）`}>
          <div className="mb-2 text-xs text-zinc-400">
            {runsLoading ? "加载中..." : `最近 ${runs.length} 条`}
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="text-zinc-500">
                <tr>
                  <th className="py-2 pr-3">ID</th>
                  <th className="py-2 pr-3">状态</th>
                  <th className="py-2 pr-3">开始</th>
                  <th className="py-2 pr-3">结束</th>
                  <th className="py-2 pr-3">signalCount</th>
                  <th className="py-2 pr-0">error</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-900">
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td className="py-2 pr-3 font-mono text-zinc-400">{r.id}</td>
                    <td className="py-2 pr-3">{r.status}</td>
                    <td className="py-2 pr-3 text-zinc-400">{toLocalTime(r.started_at)}</td>
                    <td className="py-2 pr-3 text-zinc-400">{toLocalTime(r.finished_at)}</td>
                    <td className="py-2 pr-3">{r.signal_count}</td>
                    <td className="py-2 pr-0 text-red-300">{r.error || "-"}</td>
                  </tr>
                ))}
                {runs.length === 0 ? (
                  <tr>
                    <td className="py-5 text-zinc-500" colSpan={6}>
                      暂无运行记录
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </Panel>
      ) : null}
    </div>
  );
}

function InputField(props: {
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

function SelectField(props: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: Array<{ value: string; label: string }>;
}) {
  return (
    <div>
      <div className="text-xs text-zinc-400">{props.label}</div>
      <select
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
      >
        {props.options.map((op) => (
          <option key={op.value} value={op.value}>
            {op.label}
          </option>
        ))}
      </select>
    </div>
  );
}

function ToggleField(props: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-center justify-between rounded-md border border-zinc-900 bg-zinc-950 px-3 py-2">
      <span className="text-xs text-zinc-300">{props.label}</span>
      <input
        type="checkbox"
        className="toggle toggle-sm"
        checked={props.checked}
        onChange={(e) => props.onChange(e.target.checked)}
      />
    </label>
  );
}

function JsonField(props: { label: string; value: string; onChange: (v: string) => void; rows?: number }) {
  return (
    <div>
      {props.label && <div className="text-xs text-zinc-400">{props.label}</div>}
      <textarea
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        rows={props.rows ?? 8}
        spellCheck={false}
        className={`${props.label ? "mt-1" : ""} w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-indigo-500`}
      />
    </div>
  );
}

function StatusPill(props: { ok: boolean; okText: string; noText: string }) {
  return (
    <span
      className={`inline-flex rounded-md px-2 py-0.5 font-mono text-[11px] ring-1 ${
        props.ok
          ? "bg-emerald-950/40 text-emerald-200 ring-emerald-900/60"
          : "bg-zinc-950/40 text-zinc-300 ring-zinc-800"
      }`}
    >
      {props.ok ? props.okText : props.noText}
    </span>
  );
}

function parseConfigObject(raw: string): Record<string, unknown> {
  const text = raw.trim();
  if (!text) return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    throw new Error("config 必须是合法 JSON");
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error("config 必须是 JSON 对象");
  }
  return parsed as Record<string, unknown>;
}

function parseConfigToRecord(raw: string): Record<string, unknown> {
  try {
    const text = raw.trim();
    if (!text) return {};
    const parsed = JSON.parse(text);
    if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
      return {};
    }
    return parsed as Record<string, unknown>;
  } catch {
    return {};
  }
}

function stringifyJSON(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2) ?? "{}";
  } catch {
    return "{}";
  }
}

function toPositiveInt(raw: string, fallback: number): number {
  const n = Number(raw.trim());
  if (!Number.isFinite(n) || n <= 0) return fallback;
  return Math.floor(n);
}

function toLocalTime(raw?: string): string {
  if (!raw) return "-";
  const ts = new Date(raw);
  if (Number.isNaN(ts.getTime())) return raw;
  return ts.toLocaleString();
}
