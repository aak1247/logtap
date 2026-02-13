import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useNavigate } from "react-router-dom";
import {
  createAlertContact,
  createAlertContactGroup,
  createAlertRule,
  createAlertWebhookEndpoint,
  createAlertWecomBot,
  deleteAlertContact,
  deleteAlertContactGroup,
  deleteAlertRule,
  deleteAlertWebhookEndpoint,
  deleteAlertWecomBot,
  listAlertContactGroups,
  listAlertContacts,
  listAlertDeliveries,
  listAlertRules,
  listAlertWebhookEndpoints,
  listAlertWecomBots,
  testAlertRules,
  updateAlertContact,
  updateAlertContactGroup,
  updateAlertRule,
  updateAlertWebhookEndpoint,
  updateAlertWecomBot,
  type AlertContact,
  type AlertContactGroupWithMembers,
  type AlertDelivery,
  type AlertRule,
  type AlertRulePreview,
  type AlertRuleSource,
  type AlertWebhookEndpoint,
  type AlertWecomBot,
} from "../../lib/api";
import { loadSettings, subscribeSettingsChange } from "../../lib/storage";
import { Panel } from "../components/Panel";

type AlertsTab = "contacts" | "groups" | "channels" | "rules" | "test" | "deliveries";

const tabClass =
  "px-3 py-2 rounded-lg text-sm text-zinc-300 transition-colors hover:text-zinc-100 hover:bg-zinc-900";
const tabClassActive = "bg-zinc-900 text-zinc-100";

type FieldMatchOp = "eq" | "contains" | "exists" | "in";

type RuleFieldMatchForm = {
  path: string;
  op: FieldMatchOp;
  value: string;
  values: string;
};

type RuleFormState = {
  name: string;
  enabled: boolean;
  source: AlertRuleSource;
  levels: string;
  eventNames: string;
  messageKeywords: string;
  fieldsAll: RuleFieldMatchForm[];
  windowSec: string;
  threshold: string;
  baseBackoffSec: string;
  maxBackoffSec: string;
  dedupeByMessage: boolean;
  dedupeFields: string;
  emailGroupIds: number[];
  emailContactIds: number[];
  smsGroupIds: number[];
  smsContactIds: number[];
  wecomBotIds: number[];
  webhookEndpointIds: number[];
};

function createDefaultRuleForm(): RuleFormState {
  return {
    name: "NewRule",
    enabled: true,
    source: "both",
    levels: "",
    eventNames: "",
    messageKeywords: "",
    fieldsAll: [],
    windowSec: "60",
    threshold: "1",
    baseBackoffSec: "60",
    maxBackoffSec: "3600",
    dedupeByMessage: true,
    dedupeFields: "",
    emailGroupIds: [],
    emailContactIds: [],
    smsGroupIds: [],
    smsContactIds: [],
    wecomBotIds: [],
    webhookEndpointIds: [],
  };
}

export function AlertsPage() {
  const settings = useSyncExternalStore(subscribeSettingsChange, loadSettings, loadSettings);
  const nav = useNavigate();

  const [activeTab, setActiveTab] = useState<AlertsTab>("contacts");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");

  const [contacts, setContacts] = useState<AlertContact[]>([]);
  const [groups, setGroups] = useState<AlertContactGroupWithMembers[]>([]);
  const [wecomBots, setWecomBots] = useState<AlertWecomBot[]>([]);
  const [webhookEndpoints, setWebhookEndpoints] = useState<AlertWebhookEndpoint[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [deliveries, setDeliveries] = useState<AlertDelivery[]>([]);
  const [rulePreviewItems, setRulePreviewItems] = useState<AlertRulePreview[]>([]);

  const [newContactType, setNewContactType] = useState<"email" | "sms">("email");
  const [newContactName, setNewContactName] = useState("");
  const [newContactValue, setNewContactValue] = useState("");
  const [editContactId, setEditContactId] = useState(0);
  const [editContactName, setEditContactName] = useState("");
  const [editContactValue, setEditContactValue] = useState("");

  const [newGroupType, setNewGroupType] = useState<"email" | "sms">("email");
  const [newGroupName, setNewGroupName] = useState("");
  const [newGroupMemberIds, setNewGroupMemberIds] = useState<number[]>([]);
  const [editGroupId, setEditGroupId] = useState(0);
  const [editGroupName, setEditGroupName] = useState("");
  const [editGroupMemberIds, setEditGroupMemberIds] = useState<number[]>([]);

  const [newBotName, setNewBotName] = useState("");
  const [newBotWebhook, setNewBotWebhook] = useState("");
  const [editBotId, setEditBotId] = useState(0);
  const [editBotName, setEditBotName] = useState("");
  const [editBotWebhook, setEditBotWebhook] = useState("");

  const [newEndpointName, setNewEndpointName] = useState("");
  const [newEndpointURL, setNewEndpointURL] = useState("");
  const [editEndpointId, setEditEndpointId] = useState(0);
  const [editEndpointName, setEditEndpointName] = useState("");
  const [editEndpointURL, setEditEndpointURL] = useState("");

  const [newRuleForm, setNewRuleForm] = useState<RuleFormState>(() => createDefaultRuleForm());
  const [editRuleId, setEditRuleId] = useState(0);
  const [editRuleForm, setEditRuleForm] = useState<RuleFormState>(() => createDefaultRuleForm());

  const [testSource, setTestSource] = useState<AlertRuleSource>("logs");
  const [testLevel, setTestLevel] = useState("error");
  const [testMessage, setTestMessage] = useState("boom!");
  const [testFieldsJSON, setTestFieldsJSON] = useState("{}");

  const [filterStatus, setFilterStatus] = useState<"" | "pending" | "processing" | "sent" | "failed">("");
  const [filterChannelType, setFilterChannelType] = useState<"" | "wecom" | "webhook" | "email" | "sms">("");
  const [filterRuleId, setFilterRuleId] = useState("");
  const [filterLimit, setFilterLimit] = useState("50");

  const contactsByType = useMemo(() => {
    return {
      email: contacts.filter((c) => c.type === "email"),
      sms: contacts.filter((c) => c.type === "sms"),
    };
  }, [contacts]);

  const groupsByType = useMemo(() => {
    return {
      email: groups.filter((g) => g.type === "email"),
      sms: groups.filter((g) => g.type === "sms"),
    };
  }, [groups]);

  const contactsMap = useMemo(() => {
    const out = new Map<number, AlertContact>();
    for (const c of contacts) out.set(c.id, c);
    return out;
  }, [contacts]);

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
    if (!settings.token || !settings.projectId) return;
    void loadAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings.apiBase, settings.token, settings.projectId]);

  async function loadAll() {
    try {
      setLoading(true);
      setErr("");
      const [contactsRes, groupsRes, botsRes, endpointsRes, rulesRes, deliveriesRes] =
        await Promise.all([
          listAlertContacts(settings),
          listAlertContactGroups(settings),
          listAlertWecomBots(settings),
          listAlertWebhookEndpoints(settings),
          listAlertRules(settings),
          listAlertDeliveries(settings, { limit: parseLimit(filterLimit) }),
        ]);
      setContacts(contactsRes.items);
      setGroups(groupsRes.items);
      setWecomBots(botsRes.items);
      setWebhookEndpoints(endpointsRes.items);
      setRules(rulesRes.items);
      setDeliveries(deliveriesRes.items);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }

  async function refreshDeliveries() {
    try {
      setErr("");
      const ruleId = parsePositiveInt(filterRuleId);
      const res = await listAlertDeliveries(settings, {
        status: filterStatus || undefined,
        channelType: filterChannelType || undefined,
        ruleId: ruleId || undefined,
        limit: parseLimit(filterLimit),
      });
      setDeliveries(res.items);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }

  async function runMutation(successMessage: string, fn: () => Promise<void>) {
    try {
      setBusy(successMessage);
      setErr("");
      setMsg("");
      await fn();
      setMsg(successMessage);
      await loadAll();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  }

  function startEditContact(c: AlertContact) {
    setEditContactId(c.id);
    setEditContactName(c.name);
    setEditContactValue(c.value);
  }

  function startEditGroup(g: AlertContactGroupWithMembers) {
    setEditGroupId(g.id);
    setEditGroupName(g.name);
    setEditGroupMemberIds(g.memberContactIds || []);
  }

  function startEditBot(b: AlertWecomBot) {
    setEditBotId(b.id);
    setEditBotName(b.name);
    setEditBotWebhook(b.webhook_url);
  }

  function startEditEndpoint(ep: AlertWebhookEndpoint) {
    setEditEndpointId(ep.id);
    setEditEndpointName(ep.name);
    setEditEndpointURL(ep.url);
  }

  function startEditRule(r: AlertRule) {
    setEditRuleId(r.id);
    setEditRuleForm(ruleToForm(r));
  }

  function resetRuleCreateDraft() {
    setNewRuleForm(createDefaultRuleForm());
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="text-lg font-semibold">报警</div>
          <div className="mt-1 text-sm text-zinc-400">
            配置联系人、渠道、规则，并查看投递状态。
          </div>
        </div>
        <button
          className="btn btn-md btn-outline"
          disabled={loading || Boolean(busy)}
          onClick={() => void loadAll()}
        >
          {loading ? "刷新中..." : "刷新"}
        </button>
      </div>

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

      <Panel title="功能区">
        <div className="flex flex-wrap gap-2">
          <TabButton active={activeTab === "contacts"} onClick={() => setActiveTab("contacts")}>联系人</TabButton>
          <TabButton active={activeTab === "groups"} onClick={() => setActiveTab("groups")}>联系人组</TabButton>
          <TabButton active={activeTab === "channels"} onClick={() => setActiveTab("channels")}>通知渠道</TabButton>
          <TabButton active={activeTab === "rules"} onClick={() => setActiveTab("rules")}>规则</TabButton>
          <TabButton active={activeTab === "test"} onClick={() => setActiveTab("test")}>规则测试</TabButton>
          <TabButton active={activeTab === "deliveries"} onClick={() => setActiveTab("deliveries")}>投递记录</TabButton>
        </div>
      </Panel>

      {activeTab === "contacts" ? (
        <div className="space-y-4">
          <Panel title="新建联系人">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
              <SelectField
                label="类型"
                value={newContactType}
                onChange={(v) => setNewContactType(v as "email" | "sms")}
                options={[
                  { label: "email", value: "email" },
                  { label: "sms", value: "sms" },
                ]}
              />
              <InputField
                label="名称"
                value={newContactName}
                onChange={setNewContactName}
                placeholder="ops"
              />
              <InputField
                label={newContactType === "email" ? "邮箱" : "手机号(E.164)"}
                value={newContactValue}
                onChange={setNewContactValue}
                placeholder={newContactType === "email" ? "ops@example.com" : "+8613800138000"}
              />
              <div className="flex items-end">
                <button
                  className="btn btn-md btn-primary w-full"
                  disabled={Boolean(busy)}
                  onClick={() =>
                    void runMutation("联系人已创建", async () => {
                      await createAlertContact(settings, {
                        type: newContactType,
                        name: newContactName.trim(),
                        value: newContactValue.trim(),
                      });
                      setNewContactName("");
                      setNewContactValue("");
                    })
                  }
                >
                  创建
                </button>
              </div>
            </div>
          </Panel>

          <Panel title={`联系人列表（${contacts.length}）`}>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="py-2 pr-4">ID</th>
                    <th className="py-2 pr-4">类型</th>
                    <th className="py-2 pr-4">名称</th>
                    <th className="py-2 pr-4">值</th>
                    <th className="py-2 pr-0 text-right">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {contacts.map((c) => {
                    const editing = editContactId === c.id;
                    return (
                      <tr key={c.id} className="hover:bg-zinc-900/40">
                        <td className="py-2 pr-4 font-mono text-zinc-400">{c.id}</td>
                        <td className="py-2 pr-4">{c.type}</td>
                        <td className="py-2 pr-4">
                          {editing ? (
                            <input
                              value={editContactName}
                              onChange={(e) => setEditContactName(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            <span className="text-zinc-100">{c.name || "-"}</span>
                          )}
                        </td>
                        <td className="py-2 pr-4 font-mono text-zinc-300">
                          {editing ? (
                            <input
                              value={editContactValue}
                              onChange={(e) => setEditContactValue(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            c.value
                          )}
                        </td>
                        <td className="py-2 pr-0 text-right">
                          <div className="flex justify-end gap-2">
                            {editing ? (
                              <>
                                <button
                                  className="btn btn-xs btn-outline"
                                  disabled={Boolean(busy)}
                                  onClick={() =>
                                    void runMutation("联系人已更新", async () => {
                                      await updateAlertContact(settings, c.id, {
                                        name: editContactName,
                                        value: editContactValue,
                                      });
                                      setEditContactId(0);
                                    })
                                  }
                                >
                                  保存
                                </button>
                                <button
                                  className="btn btn-xs btn-outline"
                                  onClick={() => setEditContactId(0)}
                                >
                                  取消
                                </button>
                              </>
                            ) : (
                              <button
                                className="btn btn-xs btn-outline"
                                onClick={() => startEditContact(c)}
                              >
                                编辑
                              </button>
                            )}
                            <button
                              className="btn btn-xs btn-outline"
                              disabled={Boolean(busy)}
                              onClick={() => {
                                if (!window.confirm(`确认删除联系人 #${c.id} 吗？`)) return;
                                void runMutation("联系人已删除", async () => {
                                  await deleteAlertContact(settings, c.id);
                                  if (editContactId === c.id) setEditContactId(0);
                                });
                              }}
                            >
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {contacts.length === 0 ? (
                    <tr>
                      <td className="py-6 text-sm text-zinc-500" colSpan={5}>
                        暂无联系人
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      ) : null}

      {activeTab === "groups" ? (
        <div className="space-y-4">
          <Panel title="新建联系人组">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <SelectField
                label="类型"
                value={newGroupType}
                onChange={(v) => {
                  setNewGroupType(v as "email" | "sms");
                  setNewGroupMemberIds([]);
                }}
                options={[
                  { label: "email", value: "email" },
                  { label: "sms", value: "sms" },
                ]}
              />
              <InputField
                label="组名"
                value={newGroupName}
                onChange={setNewGroupName}
                placeholder="oncall"
              />
              <div className="flex items-end">
                <button
                  className="btn btn-md btn-primary w-full"
                  disabled={Boolean(busy)}
                  onClick={() =>
                    void runMutation("联系人组已创建", async () => {
                      await createAlertContactGroup(settings, {
                        type: newGroupType,
                        name: newGroupName.trim(),
                        memberContactIds: newGroupMemberIds,
                      });
                      setNewGroupName("");
                      setNewGroupMemberIds([]);
                    })
                  }
                >
                  创建
                </button>
              </div>
            </div>
            <div className="mt-4">
              <div className="mb-2 text-xs text-zinc-400">成员（可多选）</div>
              <ContactMultiSelect
                contacts={contactsByType[newGroupType]}
                selected={newGroupMemberIds}
                onChange={setNewGroupMemberIds}
              />
            </div>
          </Panel>

          <Panel title={`联系人组列表（${groups.length}）`}>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="py-2 pr-4">ID</th>
                    <th className="py-2 pr-4">类型</th>
                    <th className="py-2 pr-4">组名</th>
                    <th className="py-2 pr-4">成员</th>
                    <th className="py-2 pr-0 text-right">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {groups.map((g) => {
                    const editing = editGroupId === g.id;
                    const members = g.memberContactIds || [];
                    return (
                      <tr key={g.id} className="hover:bg-zinc-900/40 align-top">
                        <td className="py-2 pr-4 font-mono text-zinc-400">{g.id}</td>
                        <td className="py-2 pr-4">{g.type}</td>
                        <td className="py-2 pr-4">
                          {editing ? (
                            <input
                              value={editGroupName}
                              onChange={(e) => setEditGroupName(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            <span className="text-zinc-100">{g.name}</span>
                          )}
                        </td>
                        <td className="py-2 pr-4">
                          {editing ? (
                            <ContactMultiSelect
                              contacts={contactsByType[g.type]}
                              selected={editGroupMemberIds}
                              onChange={setEditGroupMemberIds}
                            />
                          ) : (
                            <span className="text-zinc-300">{formatMembers(members, contactsMap)}</span>
                          )}
                        </td>
                        <td className="py-2 pr-0 text-right">
                          <div className="flex justify-end gap-2">
                            {editing ? (
                              <>
                                <button
                                  className="btn btn-xs btn-outline"
                                  disabled={Boolean(busy)}
                                  onClick={() =>
                                    void runMutation("联系人组已更新", async () => {
                                      await updateAlertContactGroup(settings, g.id, {
                                        name: editGroupName,
                                        memberContactIds: editGroupMemberIds,
                                      });
                                      setEditGroupId(0);
                                    })
                                  }
                                >
                                  保存
                                </button>
                                <button
                                  className="btn btn-xs btn-outline"
                                  onClick={() => setEditGroupId(0)}
                                >
                                  取消
                                </button>
                              </>
                            ) : (
                              <button
                                className="btn btn-xs btn-outline"
                                onClick={() => startEditGroup(g)}
                              >
                                编辑
                              </button>
                            )}
                            <button
                              className="btn btn-xs btn-outline"
                              disabled={Boolean(busy)}
                              onClick={() => {
                                if (!window.confirm(`确认删除联系人组 #${g.id} 吗？`)) return;
                                void runMutation("联系人组已删除", async () => {
                                  await deleteAlertContactGroup(settings, g.id);
                                  if (editGroupId === g.id) setEditGroupId(0);
                                });
                              }}
                            >
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {groups.length === 0 ? (
                    <tr>
                      <td className="py-6 text-sm text-zinc-500" colSpan={5}>
                        暂无联系人组
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      ) : null}

      {activeTab === "channels" ? (
        <div className="space-y-4">
          <Panel title="WeCom Bots">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <InputField label="名称" value={newBotName} onChange={setNewBotName} placeholder="wecom-ops" />
              <InputField
                label="Webhook URL"
                value={newBotWebhook}
                onChange={setNewBotWebhook}
                placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?..."
              />
              <div className="flex items-end">
                <button
                  className="btn btn-md btn-primary w-full"
                  disabled={Boolean(busy)}
                  onClick={() =>
                    void runMutation("WeCom Bot 已创建", async () => {
                      await createAlertWecomBot(settings, {
                        name: newBotName.trim(),
                        webhookUrl: newBotWebhook.trim(),
                      });
                      setNewBotName("");
                      setNewBotWebhook("");
                    })
                  }
                >
                  创建
                </button>
              </div>
            </div>

            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="py-2 pr-4">ID</th>
                    <th className="py-2 pr-4">名称</th>
                    <th className="py-2 pr-4">Webhook</th>
                    <th className="py-2 pr-0 text-right">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {wecomBots.map((b) => {
                    const editing = editBotId === b.id;
                    return (
                      <tr key={b.id} className="hover:bg-zinc-900/40">
                        <td className="py-2 pr-4 font-mono text-zinc-400">{b.id}</td>
                        <td className="py-2 pr-4">
                          {editing ? (
                            <input
                              value={editBotName}
                              onChange={(e) => setEditBotName(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            <span className="text-zinc-100">{b.name}</span>
                          )}
                        </td>
                        <td className="py-2 pr-4 font-mono text-zinc-300">
                          {editing ? (
                            <input
                              value={editBotWebhook}
                              onChange={(e) => setEditBotWebhook(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            <span className="block max-w-[24rem] truncate" title={b.webhook_url}>
                              {b.webhook_url}
                            </span>
                          )}
                        </td>
                        <td className="py-2 pr-0 text-right">
                          <div className="flex justify-end gap-2">
                            {editing ? (
                              <>
                                <button
                                  className="btn btn-xs btn-outline"
                                  disabled={Boolean(busy)}
                                  onClick={() =>
                                    void runMutation("WeCom Bot 已更新", async () => {
                                      await updateAlertWecomBot(settings, b.id, {
                                        name: editBotName,
                                        webhookUrl: editBotWebhook,
                                      });
                                      setEditBotId(0);
                                    })
                                  }
                                >
                                  保存
                                </button>
                                <button className="btn btn-xs btn-outline" onClick={() => setEditBotId(0)}>
                                  取消
                                </button>
                              </>
                            ) : (
                              <button className="btn btn-xs btn-outline" onClick={() => startEditBot(b)}>
                                编辑
                              </button>
                            )}
                            <button
                              className="btn btn-xs btn-outline"
                              disabled={Boolean(busy)}
                              onClick={() => {
                                if (!window.confirm(`确认删除 WeCom Bot #${b.id} 吗？`)) return;
                                void runMutation("WeCom Bot 已删除", async () => {
                                  await deleteAlertWecomBot(settings, b.id);
                                  if (editBotId === b.id) setEditBotId(0);
                                });
                              }}
                            >
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {wecomBots.length === 0 ? (
                    <tr>
                      <td className="py-6 text-sm text-zinc-500" colSpan={4}>
                        暂无 WeCom Bot
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </Panel>

          <Panel title="Webhook Endpoints">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <InputField label="名称" value={newEndpointName} onChange={setNewEndpointName} placeholder="ops-webhook" />
              <InputField
                label="URL"
                value={newEndpointURL}
                onChange={setNewEndpointURL}
                placeholder="https://example.com/hook"
              />
              <div className="flex items-end">
                <button
                  className="btn btn-md btn-primary w-full"
                  disabled={Boolean(busy)}
                  onClick={() =>
                    void runMutation("Webhook Endpoint 已创建", async () => {
                      await createAlertWebhookEndpoint(settings, {
                        name: newEndpointName.trim(),
                        url: newEndpointURL.trim(),
                      });
                      setNewEndpointName("");
                      setNewEndpointURL("");
                    })
                  }
                >
                  创建
                </button>
              </div>
            </div>

            <div className="mt-4 overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="py-2 pr-4">ID</th>
                    <th className="py-2 pr-4">名称</th>
                    <th className="py-2 pr-4">URL</th>
                    <th className="py-2 pr-0 text-right">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {webhookEndpoints.map((ep) => {
                    const editing = editEndpointId === ep.id;
                    return (
                      <tr key={ep.id} className="hover:bg-zinc-900/40">
                        <td className="py-2 pr-4 font-mono text-zinc-400">{ep.id}</td>
                        <td className="py-2 pr-4">
                          {editing ? (
                            <input
                              value={editEndpointName}
                              onChange={(e) => setEditEndpointName(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            <span className="text-zinc-100">{ep.name}</span>
                          )}
                        </td>
                        <td className="py-2 pr-4 font-mono text-zinc-300">
                          {editing ? (
                            <input
                              value={editEndpointURL}
                              onChange={(e) => setEditEndpointURL(e.target.value)}
                              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs text-zinc-100 outline-none focus:border-indigo-500"
                            />
                          ) : (
                            <span className="block max-w-[24rem] truncate" title={ep.url}>
                              {ep.url}
                            </span>
                          )}
                        </td>
                        <td className="py-2 pr-0 text-right">
                          <div className="flex justify-end gap-2">
                            {editing ? (
                              <>
                                <button
                                  className="btn btn-xs btn-outline"
                                  disabled={Boolean(busy)}
                                  onClick={() =>
                                    void runMutation("Webhook Endpoint 已更新", async () => {
                                      await updateAlertWebhookEndpoint(settings, ep.id, {
                                        name: editEndpointName,
                                        url: editEndpointURL,
                                      });
                                      setEditEndpointId(0);
                                    })
                                  }
                                >
                                  保存
                                </button>
                                <button
                                  className="btn btn-xs btn-outline"
                                  onClick={() => setEditEndpointId(0)}
                                >
                                  取消
                                </button>
                              </>
                            ) : (
                              <button className="btn btn-xs btn-outline" onClick={() => startEditEndpoint(ep)}>
                                编辑
                              </button>
                            )}
                            <button
                              className="btn btn-xs btn-outline"
                              disabled={Boolean(busy)}
                              onClick={() => {
                                if (!window.confirm(`确认删除 Webhook Endpoint #${ep.id} 吗？`)) return;
                                void runMutation("Webhook Endpoint 已删除", async () => {
                                  await deleteAlertWebhookEndpoint(settings, ep.id);
                                  if (editEndpointId === ep.id) setEditEndpointId(0);
                                });
                              }}
                            >
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {webhookEndpoints.length === 0 ? (
                    <tr>
                      <td className="py-6 text-sm text-zinc-500" colSpan={4}>
                        暂无 Webhook Endpoint
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      ) : null}

      {activeTab === "rules" ? (
        <div className="space-y-4">
          <Panel title="新建规则（可视化）">
            <RuleFormEditor
              form={newRuleForm}
              onChange={setNewRuleForm}
              contactsByType={contactsByType}
              groupsByType={groupsByType}
              wecomBots={wecomBots}
              webhookEndpoints={webhookEndpoints}
            />
            <div className="mt-3 flex justify-end gap-2">
              <button
                className="btn btn-md btn-outline"
                disabled={Boolean(busy)}
                onClick={resetRuleCreateDraft}
              >
                重置
              </button>
              <button
                className="btn btn-md btn-primary"
                disabled={Boolean(busy)}
                onClick={() =>
                  void runMutation("规则已创建", async () => {
                    await createAlertRule(settings, buildRulePayloadFromForm(newRuleForm));
                    resetRuleCreateDraft();
                  })
                }
              >
                创建规则
              </button>
            </div>
          </Panel>

          <Panel title={`规则列表（${rules.length}）`}>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="py-2 pr-4">ID</th>
                    <th className="py-2 pr-4">名称</th>
                    <th className="py-2 pr-4">启用</th>
                    <th className="py-2 pr-4">Source</th>
                    <th className="py-2 pr-4">Targets 摘要</th>
                    <th className="py-2 pr-0 text-right">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {rules.map((r) => {
                    const editing = editRuleId === r.id;
                    return (
                      <tr key={r.id} className="hover:bg-zinc-900/40 align-top">
                        <td className="py-2 pr-4 font-mono text-zinc-400">{r.id}</td>
                        <td className="py-2 pr-4 text-zinc-100">{r.name}</td>
                        <td className="py-2 pr-4">
                          <StatusPill ok={r.enabled} okText="enabled" noText="disabled" />
                        </td>
                        <td className="py-2 pr-4">{r.source}</td>
                        <td className="py-2 pr-4 text-zinc-300">{formatTargetsSummary(r.targets)}</td>
                        <td className="py-2 pr-0 text-right">
                          <div className="flex justify-end gap-2">
                            <button
                              className="btn btn-xs btn-outline"
                              onClick={() => {
                                if (editing) {
                                  setEditRuleId(0);
                                  return;
                                }
                                startEditRule(r);
                              }}
                            >
                              {editing ? "收起" : "编辑"}
                            </button>
                            <button
                              className="btn btn-xs btn-outline"
                              disabled={Boolean(busy)}
                              onClick={() => {
                                if (!window.confirm(`确认删除规则 #${r.id} 吗？`)) return;
                                void runMutation("规则已删除", async () => {
                                  await deleteAlertRule(settings, r.id);
                                  if (editRuleId === r.id) setEditRuleId(0);
                                });
                              }}
                            >
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {rules.map((r) => {
                    if (editRuleId !== r.id) return null;
                    return (
                      <tr key={`edit-form-${r.id}`} className="bg-zinc-950/40">
                        <td className="py-2" colSpan={6}>
                          <div className="rounded-lg border border-zinc-900 bg-zinc-950 p-3">
                            <RuleFormEditor
                              form={editRuleForm}
                              onChange={setEditRuleForm}
                              contactsByType={contactsByType}
                              groupsByType={groupsByType}
                              wecomBots={wecomBots}
                              webhookEndpoints={webhookEndpoints}
                            />
                            <div className="mt-3 flex justify-end gap-2">
                              <button className="btn btn-xs btn-outline" onClick={() => setEditRuleId(0)}>
                                取消
                              </button>
                              <button
                                className="btn btn-xs btn-primary"
                                disabled={Boolean(busy)}
                                onClick={() =>
                                  void runMutation("规则已更新", async () => {
                                    await updateAlertRule(settings, r.id, buildRulePayloadFromForm(editRuleForm));
                                    setEditRuleId(0);
                                  })
                                }
                              >
                                保存
                              </button>
                            </div>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                  {rules.length === 0 ? (
                    <tr>
                      <td className="py-6 text-sm text-zinc-500" colSpan={6}>
                        暂无规则
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      ) : null}

      {activeTab === "test" ? (
        <div className="space-y-4">
          <Panel title="规则测试（dry-run）">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <SelectField
                label="Source"
                value={testSource}
                onChange={(v) => setTestSource(v as AlertRuleSource)}
                options={[
                  { label: "logs", value: "logs" },
                  { label: "events", value: "events" },
                  { label: "both", value: "both" },
                ]}
              />
              <InputField label="Level" value={testLevel} onChange={setTestLevel} placeholder="error" />
              <InputField label="Message" value={testMessage} onChange={setTestMessage} placeholder="boom!" />
            </div>
            <div className="mt-3">
              <JsonField label="Fields JSON" value={testFieldsJSON} onChange={setTestFieldsJSON} rows={8} />
            </div>
            <div className="mt-3 flex justify-end">
              <button
                className="btn btn-md btn-primary"
                disabled={Boolean(busy)}
                onClick={() =>
                  void runMutation("规则测试完成", async () => {
                    const res = await testAlertRules(settings, {
                      source: testSource,
                      level: testLevel.trim(),
                      message: testMessage.trim(),
                      fields: parseJSONObject("fields", testFieldsJSON),
                    });
                    setRulePreviewItems(res.items || []);
                  })
                }
              >
                测试
              </button>
            </div>
          </Panel>

          <Panel title={`测试结果（${rulePreviewItems.length}）`}>
            <div className="space-y-3">
              {rulePreviewItems.map((it) => (
                <div key={it.ruleId} className="rounded-lg border border-zinc-900 bg-zinc-950/40 p-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-xs text-zinc-400">#{it.ruleId}</span>
                    <span className="text-sm font-semibold text-zinc-100">{it.ruleName}</span>
                    <StatusPill ok={Boolean(it.matched)} okText="matched" noText="not matched" />
                    <StatusPill ok={Boolean(it.willEnqueue)} okText="will enqueue" noText="suppressed" />
                  </div>
                  {it.suppressedReason ? (
                    <div className="mt-2 text-xs text-amber-300">
                      {it.suppressedReason}: {it.suppressedMessage || ""}
                    </div>
                  ) : null}
                  {it.deliveries && it.deliveries.length > 0 ? (
                    <div className="mt-2 overflow-x-auto">
                      <table className="w-full text-left text-xs">
                        <thead className="text-zinc-500">
                          <tr>
                            <th className="py-1.5 pr-3">channel</th>
                            <th className="py-1.5 pr-3">target</th>
                            <th className="py-1.5 pr-0">title</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-zinc-900">
                          {it.deliveries.map((d, idx) => (
                            <tr key={`${it.ruleId}-${idx}`}>
                              <td className="py-1.5 pr-3">{d.channelType}</td>
                              <td className="py-1.5 pr-3 font-mono text-zinc-300">{d.target}</td>
                              <td className="py-1.5 pr-0 text-zinc-300">{d.title}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : null}
                </div>
              ))}
              {rulePreviewItems.length === 0 ? (
                <div className="text-sm text-zinc-500">暂无测试结果</div>
              ) : null}
            </div>
          </Panel>
        </div>
      ) : null}

      {activeTab === "deliveries" ? (
        <div className="space-y-4">
          <Panel
            title="查询条件"
            right={
              <button
                className="btn btn-md btn-primary"
                disabled={Boolean(busy)}
                onClick={() => void refreshDeliveries()}
              >
                查询
              </button>
            }
          >
            <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
              <SelectField
                label="状态"
                value={filterStatus}
                onChange={(v) => setFilterStatus(v as "" | "pending" | "processing" | "sent" | "failed")}
                options={[
                  { label: "全部", value: "" },
                  { label: "pending", value: "pending" },
                  { label: "processing", value: "processing" },
                  { label: "sent", value: "sent" },
                  { label: "failed", value: "failed" },
                ]}
              />
              <SelectField
                label="渠道"
                value={filterChannelType}
                onChange={(v) => setFilterChannelType(v as "" | "wecom" | "webhook" | "email" | "sms")}
                options={[
                  { label: "全部", value: "" },
                  { label: "wecom", value: "wecom" },
                  { label: "webhook", value: "webhook" },
                  { label: "email", value: "email" },
                  { label: "sms", value: "sms" },
                ]}
              />
              <InputField label="ruleId" value={filterRuleId} onChange={setFilterRuleId} placeholder="1" />
              <InputField label="limit" value={filterLimit} onChange={setFilterLimit} placeholder="50" />
            </div>
          </Panel>

          <Panel title={`投递记录（${deliveries.length}）`}>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="py-2 pr-4">ID</th>
                    <th className="py-2 pr-4">规则</th>
                    <th className="py-2 pr-4">状态</th>
                    <th className="py-2 pr-4">渠道</th>
                    <th className="py-2 pr-4">目标</th>
                    <th className="py-2 pr-4">尝试</th>
                    <th className="py-2 pr-4">下次重试</th>
                    <th className="py-2 pr-0">错误</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {deliveries.map((d) => (
                    <tr key={d.id} className="hover:bg-zinc-900/40 align-top">
                      <td className="py-2 pr-4 font-mono text-zinc-400">{d.id}</td>
                      <td className="py-2 pr-4 font-mono text-zinc-300">{d.rule_id}</td>
                      <td className="py-2 pr-4">
                        <StatusPill ok={d.status === "sent"} okText={d.status} noText={d.status} />
                      </td>
                      <td className="py-2 pr-4">{d.channel_type}</td>
                      <td className="py-2 pr-4 font-mono text-zinc-300">
                        <span className="block max-w-[18rem] truncate" title={d.target}>
                          {d.target}
                        </span>
                      </td>
                      <td className="py-2 pr-4">{d.attempts}</td>
                      <td className="py-2 pr-4 text-zinc-400">
                        {d.next_attempt_at ? new Date(d.next_attempt_at).toLocaleString() : "-"}
                      </td>
                      <td className="py-2 pr-0 text-red-300">{d.last_error || "-"}</td>
                    </tr>
                  ))}
                  {deliveries.length === 0 ? (
                    <tr>
                      <td className="py-6 text-sm text-zinc-500" colSpan={8}>
                        暂无投递记录
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>
      ) : null}
    </div>
  );
}

function TabButton(props: { active: boolean; onClick: () => void; children: string }) {
  return (
    <button
      className={`${tabClass} ${props.active ? tabClassActive : ""}`}
      onClick={props.onClick}
    >
      {props.children}
    </button>
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
  options: Array<{ label: string; value: string }>;
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
      <div className="text-xs text-zinc-400">{props.label}</div>
      <textarea
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        rows={props.rows ?? 10}
        spellCheck={false}
        className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-indigo-500"
      />
    </div>
  );
}

function RuleFormEditor(props: {
  form: RuleFormState;
  onChange: (next: RuleFormState) => void;
  contactsByType: { email: AlertContact[]; sms: AlertContact[] };
  groupsByType: { email: AlertContactGroupWithMembers[]; sms: AlertContactGroupWithMembers[] };
  wecomBots: AlertWecomBot[];
  webhookEndpoints: AlertWebhookEndpoint[];
}) {
  const f = props.form;
  const set = (patch: Partial<RuleFormState>) => props.onChange({ ...f, ...patch });
  const fieldRows = f.fieldsAll;

  const setFieldRow = (idx: number, patch: Partial<RuleFieldMatchForm>) => {
    const next = fieldRows.map((row, i) => (i === idx ? { ...row, ...patch } : row));
    set({ fieldsAll: next });
  };

  const removeFieldRow = (idx: number) => {
    set({ fieldsAll: fieldRows.filter((_, i) => i !== idx) });
  };

  const addFieldRow = () => {
    set({
      fieldsAll: [...fieldRows, { path: "", op: "eq", value: "", values: "" }],
    });
  };

  const emailContactOptions = props.contactsByType.email.map((c) => ({
    id: c.id,
    label: `#${c.id} ${c.name || c.value}`,
  }));
  const smsContactOptions = props.contactsByType.sms.map((c) => ({
    id: c.id,
    label: `#${c.id} ${c.name || c.value}`,
  }));
  const emailGroupOptions = props.groupsByType.email.map((g) => ({
    id: g.id,
    label: `#${g.id} ${g.name}`,
  }));
  const smsGroupOptions = props.groupsByType.sms.map((g) => ({
    id: g.id,
    label: `#${g.id} ${g.name}`,
  }));
  const wecomOptions = props.wecomBots.map((b) => ({ id: b.id, label: `#${b.id} ${b.name}` }));
  const webhookOptions = props.webhookEndpoints.map((ep) => ({ id: ep.id, label: `#${ep.id} ${ep.name}` }));

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        <InputField label="规则名" value={f.name} onChange={(v) => set({ name: v })} placeholder="BoomRule" />
        <SelectField
          label="Source"
          value={f.source}
          onChange={(v) => set({ source: v as AlertRuleSource })}
          options={[
            { label: "logs", value: "logs" },
            { label: "events", value: "events" },
            { label: "both", value: "both" },
          ]}
        />
        <ToggleField label="启用" checked={f.enabled} onChange={(v) => set({ enabled: v })} />
      </div>

      <div className="rounded-lg border border-zinc-900 bg-zinc-950/40 p-3">
        <div className="mb-2 text-sm font-semibold text-zinc-100">匹配条件</div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          <InputField
            label="Levels（逗号分隔）"
            value={f.levels}
            onChange={(v) => set({ levels: v })}
            placeholder="error,fatal"
          />
          <InputField
            label="Event Names（逗号分隔）"
            value={f.eventNames}
            onChange={(v) => set({ eventNames: v })}
            placeholder="signup,pay_success"
          />
          <InputField
            label="Message Keywords（逗号分隔）"
            value={f.messageKeywords}
            onChange={(v) => set({ messageKeywords: v })}
            placeholder="boom,timeout"
          />
        </div>

        <div className="mt-3">
          <div className="mb-2 flex items-center justify-between">
            <div className="text-xs text-zinc-400">Fields All（全部命中）</div>
            <button className="btn btn-xs btn-outline" onClick={addFieldRow}>
              新增字段条件
            </button>
          </div>
          <div className="space-y-2">
            {fieldRows.map((row, idx) => (
              <div key={idx} className="grid grid-cols-1 gap-2 rounded border border-zinc-900 p-2 md:grid-cols-12">
                <div className="md:col-span-4">
                  <InputField
                    label="path"
                    value={row.path}
                    onChange={(v) => setFieldRow(idx, { path: v })}
                    placeholder="exception.values.0.type"
                  />
                </div>
                <div className="md:col-span-2">
                  <SelectField
                    label="op"
                    value={row.op}
                    onChange={(v) => setFieldRow(idx, { op: v as FieldMatchOp })}
                    options={[
                      { label: "eq", value: "eq" },
                      { label: "contains", value: "contains" },
                      { label: "exists", value: "exists" },
                      { label: "in", value: "in" },
                    ]}
                  />
                </div>
                <div className="md:col-span-5">
                  {row.op === "exists" ? (
                    <div className="rounded-md border border-zinc-900 bg-zinc-950 px-3 py-2 text-xs text-zinc-500">
                      exists 无需 value
                    </div>
                  ) : row.op === "in" ? (
                    <InputField
                      label="values（逗号分隔）"
                      value={row.values}
                      onChange={(v) => setFieldRow(idx, { values: v })}
                      placeholder="prod,staging"
                    />
                  ) : (
                    <InputField
                      label="value"
                      value={row.value}
                      onChange={(v) => setFieldRow(idx, { value: v })}
                      placeholder="TypeError"
                    />
                  )}
                </div>
                <div className="md:col-span-1">
                  <div className="text-xs text-transparent">x</div>
                  <button className="btn btn-xs btn-outline w-full" onClick={() => removeFieldRow(idx)}>
                    删除
                  </button>
                </div>
              </div>
            ))}
            {fieldRows.length === 0 ? <div className="text-xs text-zinc-500">未配置字段条件</div> : null}
          </div>
        </div>
      </div>

      <div className="rounded-lg border border-zinc-900 bg-zinc-950/40 p-3">
        <div className="mb-2 text-sm font-semibold text-zinc-100">去重与退避</div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-4">
          <InputField label="windowSec" value={f.windowSec} onChange={(v) => set({ windowSec: v })} placeholder="60" />
          <InputField label="threshold" value={f.threshold} onChange={(v) => set({ threshold: v })} placeholder="1" />
          <InputField
            label="baseBackoffSec"
            value={f.baseBackoffSec}
            onChange={(v) => set({ baseBackoffSec: v })}
            placeholder="60"
          />
          <InputField
            label="maxBackoffSec"
            value={f.maxBackoffSec}
            onChange={(v) => set({ maxBackoffSec: v })}
            placeholder="3600"
          />
        </div>
        <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
          <ToggleField
            label="Dedupe By Message"
            checked={f.dedupeByMessage}
            onChange={(v) => set({ dedupeByMessage: v })}
          />
          <InputField
            label="Dedupe Fields（逗号分隔）"
            value={f.dedupeFields}
            onChange={(v) => set({ dedupeFields: v })}
            placeholder="user.id,device.id"
          />
        </div>
      </div>

      <div className="rounded-lg border border-zinc-900 bg-zinc-950/40 p-3">
        <div className="mb-2 text-sm font-semibold text-zinc-100">通知目标</div>
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <IdMultiSelect
            label="Email Contacts"
            items={emailContactOptions}
            selected={f.emailContactIds}
            onChange={(ids) => set({ emailContactIds: ids })}
          />
          <IdMultiSelect
            label="Email Groups"
            items={emailGroupOptions}
            selected={f.emailGroupIds}
            onChange={(ids) => set({ emailGroupIds: ids })}
          />
          <IdMultiSelect
            label="SMS Contacts"
            items={smsContactOptions}
            selected={f.smsContactIds}
            onChange={(ids) => set({ smsContactIds: ids })}
          />
          <IdMultiSelect
            label="SMS Groups"
            items={smsGroupOptions}
            selected={f.smsGroupIds}
            onChange={(ids) => set({ smsGroupIds: ids })}
          />
          <IdMultiSelect
            label="WeCom Bots"
            items={wecomOptions}
            selected={f.wecomBotIds}
            onChange={(ids) => set({ wecomBotIds: ids })}
          />
          <IdMultiSelect
            label="Webhook Endpoints"
            items={webhookOptions}
            selected={f.webhookEndpointIds}
            onChange={(ids) => set({ webhookEndpointIds: ids })}
          />
        </div>
      </div>
    </div>
  );
}

function IdMultiSelect(props: {
  label: string;
  items: Array<{ id: number; label: string }>;
  selected: number[];
  onChange: (ids: number[]) => void;
}) {
  return (
    <div>
      <div className="mb-1 text-xs text-zinc-400">{props.label}</div>
      <div className="max-h-44 overflow-auto rounded-md border border-zinc-900 bg-zinc-950/30 p-2">
        <div className="grid grid-cols-1 gap-1">
          {props.items.map((item) => {
            const checked = props.selected.includes(item.id);
            return (
              <label key={item.id} className="flex items-center gap-2 rounded px-2 py-1 hover:bg-zinc-900/60">
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={(e) => {
                    if (e.target.checked) {
                      props.onChange(uniqueInts([...props.selected, item.id]));
                    } else {
                      props.onChange(props.selected.filter((x) => x !== item.id));
                    }
                  }}
                />
                <span className="text-xs text-zinc-200">{item.label}</span>
              </label>
            );
          })}
        </div>
        {props.items.length === 0 ? <div className="text-xs text-zinc-500">暂无可选项</div> : null}
      </div>
    </div>
  );
}

function ContactMultiSelect(props: {
  contacts: AlertContact[];
  selected: number[];
  onChange: (ids: number[]) => void;
}) {
  return (
    <div className="max-h-48 overflow-auto rounded-md border border-zinc-900 bg-zinc-950/30 p-2">
      <div className="grid grid-cols-1 gap-1 md:grid-cols-2">
        {props.contacts.map((c) => {
          const checked = props.selected.includes(c.id);
          return (
            <label key={c.id} className="flex items-center gap-2 rounded px-2 py-1 hover:bg-zinc-900/60">
              <input
                type="checkbox"
                checked={checked}
                onChange={(e) => {
                  if (e.target.checked) {
                    props.onChange(uniqueInts([...props.selected, c.id]));
                  } else {
                    props.onChange(props.selected.filter((x) => x !== c.id));
                  }
                }}
              />
              <span className="text-xs text-zinc-200">#{c.id} {c.name || c.value}</span>
            </label>
          );
        })}
      </div>
      {props.contacts.length === 0 ? <div className="text-xs text-zinc-500">无可选联系人</div> : null}
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

function parseJSONObject(label: string, raw: string): Record<string, unknown> {
  const text = raw.trim();
  if (!text) return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    throw new Error(`${label} 不是合法 JSON`);
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error(`${label} 必须是 JSON 对象`);
  }
  return parsed as Record<string, unknown>;
}

function buildRulePayloadFromForm(form: RuleFormState): {
  name: string;
  enabled: boolean;
  source: AlertRuleSource;
  match: Record<string, unknown>;
  repeat: Record<string, unknown>;
  targets: Record<string, unknown>;
} {
  const name = form.name.trim();
  if (!name) throw new Error("规则名不能为空");

  const match: Record<string, unknown> = {};
  const levels = splitCSV(form.levels);
  const eventNames = splitCSV(form.eventNames);
  const messageKeywords = splitCSV(form.messageKeywords);
  if (levels.length > 0) match.levels = levels;
  if (eventNames.length > 0) match.eventNames = eventNames;
  if (messageKeywords.length > 0) match.messageKeywords = messageKeywords;

  const fieldsAll: Array<Record<string, unknown>> = [];
  for (const f of form.fieldsAll) {
    const path = f.path.trim();
    if (!path) continue;
    if (f.op === "exists") {
      fieldsAll.push({ path, op: "exists" });
      continue;
    }
    if (f.op === "in") {
      const values = splitCSV(f.values);
      if (values.length === 0) continue;
      fieldsAll.push({ path, op: "in", values });
      continue;
    }
    fieldsAll.push({ path, op: f.op, value: f.value });
  }
  if (fieldsAll.length > 0) match.fieldsAll = fieldsAll;

  const repeat: Record<string, unknown> = {};
  const windowSec = parseIntOrZero(form.windowSec);
  const threshold = parseIntOrZero(form.threshold);
  const baseBackoffSec = parseIntOrZero(form.baseBackoffSec);
  const maxBackoffSec = parseIntOrZero(form.maxBackoffSec);
  if (windowSec > 0) repeat.windowSec = windowSec;
  if (threshold > 0) repeat.threshold = threshold;
  if (baseBackoffSec > 0) repeat.baseBackoffSec = baseBackoffSec;
  if (maxBackoffSec > 0) repeat.maxBackoffSec = maxBackoffSec;
  repeat.dedupeByMessage = form.dedupeByMessage;
  const dedupeFields = splitCSV(form.dedupeFields);
  if (dedupeFields.length > 0) repeat.dedupeFields = dedupeFields;

  const targets: Record<string, unknown> = {};
  if (form.emailGroupIds.length > 0) targets.emailGroupIds = uniqueInts(form.emailGroupIds);
  if (form.emailContactIds.length > 0) targets.emailContactIds = uniqueInts(form.emailContactIds);
  if (form.smsGroupIds.length > 0) targets.smsGroupIds = uniqueInts(form.smsGroupIds);
  if (form.smsContactIds.length > 0) targets.smsContactIds = uniqueInts(form.smsContactIds);
  if (form.wecomBotIds.length > 0) targets.wecomBotIds = uniqueInts(form.wecomBotIds);
  if (form.webhookEndpointIds.length > 0) targets.webhookEndpointIds = uniqueInts(form.webhookEndpointIds);

  return {
    name,
    enabled: form.enabled,
    source: form.source,
    match,
    repeat,
    targets,
  };
}

function ruleToForm(rule: AlertRule): RuleFormState {
  const match = asRecord(rule.match);
  const repeat = asRecord(rule.repeat);
  const targets = asRecord(rule.targets);

  const fieldsAllRaw = asArray(match.fieldsAll);
  const fieldsAll: RuleFieldMatchForm[] = fieldsAllRaw
    .map((item) => {
      const row = asRecord(item);
      const path = toText(row.path);
      if (!path) return null;
      const op = toFieldOp(row.op);
      const value = toText(row.value);
      const values = toCSV(asArray(row.values).map((v) => toText(v)).filter((v) => v !== ""));
      return { path, op, value, values };
    })
    .filter((x): x is RuleFieldMatchForm => Boolean(x));

  return {
    name: rule.name || `rule-${rule.id}`,
    enabled: Boolean(rule.enabled),
    source: rule.source === "logs" || rule.source === "events" || rule.source === "both" ? rule.source : "both",
    levels: toCSV(asArray(match.levels).map((v) => toText(v)).filter((v) => v !== "")),
    eventNames: toCSV(asArray(match.eventNames).map((v) => toText(v)).filter((v) => v !== "")),
    messageKeywords: toCSV(asArray(match.messageKeywords).map((v) => toText(v)).filter((v) => v !== "")),
    fieldsAll,
    windowSec: String(intFromUnknown(repeat.windowSec, 60)),
    threshold: String(intFromUnknown(repeat.threshold, 1)),
    baseBackoffSec: String(intFromUnknown(repeat.baseBackoffSec, 60)),
    maxBackoffSec: String(intFromUnknown(repeat.maxBackoffSec, 3600)),
    dedupeByMessage: boolFromUnknown(repeat.dedupeByMessage, true),
    dedupeFields: toCSV(asArray(repeat.dedupeFields).map((v) => toText(v)).filter((v) => v !== "")),
    emailGroupIds: uniqueInts(asArray(targets.emailGroupIds).map((v) => parseIntOrZero(toText(v))).filter((v) => v > 0)),
    emailContactIds: uniqueInts(asArray(targets.emailContactIds).map((v) => parseIntOrZero(toText(v))).filter((v) => v > 0)),
    smsGroupIds: uniqueInts(asArray(targets.smsGroupIds).map((v) => parseIntOrZero(toText(v))).filter((v) => v > 0)),
    smsContactIds: uniqueInts(asArray(targets.smsContactIds).map((v) => parseIntOrZero(toText(v))).filter((v) => v > 0)),
    wecomBotIds: uniqueInts(asArray(targets.wecomBotIds).map((v) => parseIntOrZero(toText(v))).filter((v) => v > 0)),
    webhookEndpointIds: uniqueInts(
      asArray(targets.webhookEndpointIds).map((v) => parseIntOrZero(toText(v))).filter((v) => v > 0),
    ),
  };
}

function asRecord(v: unknown): Record<string, unknown> {
  if (!v || Array.isArray(v) || typeof v !== "object") return {};
  return v as Record<string, unknown>;
}

function asArray(v: unknown): unknown[] {
  return Array.isArray(v) ? v : [];
}

function toText(v: unknown): string {
  if (typeof v === "string") return v.trim();
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  if (v == null) return "";
  try {
    return JSON.stringify(v);
  } catch {
    return "";
  }
}

function toFieldOp(v: unknown): FieldMatchOp {
  const s = toText(v).toLowerCase();
  if (s === "contains" || s === "exists" || s === "in") return s;
  return "eq";
}

function splitCSV(raw: string): string[] {
  return raw
    .split(",")
    .map((x) => x.trim())
    .filter((x) => x !== "");
}

function toCSV(items: string[]): string {
  return items.join(", ");
}

function parseIntOrZero(raw: string): number {
  const n = Number(raw.trim());
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.floor(n);
}

function intFromUnknown(v: unknown, defaultValue: number): number {
  const n = parseIntOrZero(toText(v));
  return n > 0 ? n : defaultValue;
}

function boolFromUnknown(v: unknown, defaultValue: boolean): boolean {
  if (typeof v === "boolean") return v;
  if (typeof v === "string") {
    const s = v.trim().toLowerCase();
    if (s === "true") return true;
    if (s === "false") return false;
  }
  return defaultValue;
}

function formatTargetsSummary(targets: Record<string, unknown>): string {
  const email = countArray(targets.emailGroupIds) + countArray(targets.emailContactIds);
  const sms = countArray(targets.smsGroupIds) + countArray(targets.smsContactIds);
  const wecom = countArray(targets.wecomBotIds);
  const webhook = countArray(targets.webhookEndpointIds);
  return `email:${email} sms:${sms} wecom:${wecom} webhook:${webhook}`;
}

function countArray(v: unknown): number {
  return Array.isArray(v) ? v.length : 0;
}

function parsePositiveInt(raw: string): number {
  const n = Number(raw.trim());
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.floor(n);
}

function parseLimit(raw: string): number {
  const n = parsePositiveInt(raw);
  if (n <= 0) return 50;
  if (n > 200) return 200;
  return n;
}

function uniqueInts(ids: number[]): number[] {
  const seen = new Set<number>();
  const out: number[] = [];
  for (const id of ids) {
    if (!Number.isInteger(id) || id <= 0 || seen.has(id)) continue;
    seen.add(id);
    out.push(id);
  }
  return out;
}

function formatMembers(ids: number[], map: Map<number, AlertContact>): string {
  if (!ids || ids.length === 0) return "-";
  return ids
    .map((id) => {
      const c = map.get(id);
      if (!c) return `#${id}`;
      return `${c.name || c.value}(#${id})`;
    })
    .join(", ");
}
