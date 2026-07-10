import { useEffect, useMemo, useState } from "react";
import {
  previewCloudMigration,
  startCloudMigration,
  type MigrationPreview,
  type MigrationPreviewProject,
} from "../../lib/api";
import { loadSettings } from "../../lib/storage";

export function CloudMigrationPanel() {
  const settings = useMemo(() => loadSettings(), []);
  const [preview, setPreview] = useState<MigrationPreview | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [cloudURL, setCloudURL] = useState(
    typeof window !== "undefined" ? `${window.location.protocol}//${window.location.hostname}:5176` : "http://127.0.0.1:5176",
  );
  const [cloudToken, setCloudToken] = useState("");
  const [orgID, setOrgID] = useState("");
  const [days, setDays] = useState("30");
  const [includeSecrets, setIncludeSecrets] = useState(true);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ job_id: string; status: string; status_url?: string } | null>(null);

  useEffect(() => {
    if (!settings.token) {
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        setErr("");
        const res = await previewCloudMigration(settings, buildRange(days));
        if (cancelled) return;
        setPreview(res);
        if (res.cloud_default_url) setCloudURL(res.cloud_default_url);
        setSelected(new Set(res.projects.map((p) => p.id)));
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.token, days]);

  const selectedProjects = preview?.projects.filter((p) => selected.has(p.id)) ?? [];
  const selectedLogs = selectedProjects.reduce((sum, p) => sum + p.logs, 0);
  const selectedEvents = selectedProjects.reduce((sum, p) => sum + p.events, 0);
  const projects = preview?.projects ?? [];
  const allSelected = projects.length > 0 && selected.size === projects.length;

  async function start() {
    if (!cloudToken.trim()) {
      setErr("需要填写 Cloud Token");
      return;
    }
    try {
      setBusy(true);
      setErr("");
      setResult(null);
      const range = buildRange(days);
      const res = await startCloudMigration(settings, {
        cloud_api_base: cloudURL.trim(),
        cloud_token: cloudToken.trim(),
        org_id: orgID.trim() ? Number(orgID.trim()) : undefined,
        project_ids: Array.from(selected),
        include_notification_secrets: includeSecrets,
        ...range,
      });
      setResult(res);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-lg font-semibold">上云迁移</div>
          <div className="mt-1 max-w-2xl text-sm text-zinc-400">
            将本地项目推送到 logtap cloud，减少运维负担和成本。
          </div>
        </div>
        <a className="btn btn-sm btn-outline" href={`${cloudURL.replace(/\/+$/, "")}/login`} target="_blank" rel="noreferrer">
          打开 Cloud
        </a>
      </div>

      {err ? <div className="rounded-md border border-red-900/60 bg-red-950/40 p-3 text-sm text-red-200">{err}</div> : null}
      {result ? (
        <div className="rounded-md border border-emerald-900/60 bg-emerald-950/30 p-3 text-sm text-emerald-100">
          已创建导入任务：<span className="font-mono">{result.job_id}</span>
          <div className="mt-2">
            <a
              className="btn btn-xs btn-outline border-emerald-900/60 text-emerald-100 hover:bg-emerald-950/40"
              href={cloudImportJobURL(cloudURL, result.job_id)}
              target="_blank"
              rel="noreferrer"
            >
              查看导入进度
            </a>
          </div>
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <section className="overflow-hidden rounded-lg border border-zinc-900 bg-zinc-950 lg:col-span-2">
          <div className="border-b border-zinc-900 px-4 py-3">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="text-sm font-semibold">项目和数据范围</div>
                <div className="mt-1 text-xs text-zinc-500">选择要导入云端的本地项目，并限定日志/事件数据的时间范围。</div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  className="btn btn-xs btn-outline"
                  disabled={projects.length === 0 || allSelected}
                  onClick={() => setSelected(new Set(projects.map((p) => p.id)))}
                >
                  全选
                </button>
                <button
                  className="btn btn-xs btn-outline"
                  disabled={selected.size === 0}
                  onClick={() => setSelected(new Set())}
                >
                  清空
                </button>
              </div>
            </div>
          </div>

          <div className="space-y-4 p-4">
            <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(12rem,16rem)_1fr]">
              <label className="field-label">
                数据范围
                <select className="input input-compact mt-1" value={days} onChange={(e) => setDays(e.target.value)}>
                  <option value="7">最近 7 天</option>
                  <option value="30">最近 30 天</option>
                  <option value="90">最近 90 天</option>
                  <option value="all">全部</option>
                </select>
                <span className="field-hint">切换后会重新计算可迁移数据量。</span>
              </label>

              <label className="flex min-h-[4.75rem] cursor-pointer items-start gap-3 rounded-md border border-zinc-900 bg-zinc-950/60 p-3 transition-colors hover:border-zinc-800 hover:bg-zinc-900/30">
                <input
                  className="check-input mt-0.5"
                  type="checkbox"
                  checked={includeSecrets}
                  onChange={(e) => setIncludeSecrets(e.target.checked)}
                />
                <span>
                  <span className="block text-sm font-medium text-zinc-200">迁移通知 Webhook Secret</span>
                  <span className="mt-1 block text-xs leading-5 text-zinc-500">
                    保留本地通知渠道的 webhook secret；本地用户、密码、session 和本地 project key 不会迁移。
                  </span>
                </span>
              </label>
            </div>

            <div className="overflow-hidden rounded-md border border-zinc-900">
              <table className="w-full table-fixed text-left text-sm">
                <thead className="bg-zinc-900/70 text-xs text-zinc-400">
                  <tr>
                    <th className="w-12 px-3 py-2"></th>
                    <th className="px-3 py-2">项目</th>
                    <th className="w-28 px-3 py-2 text-right">日志</th>
                    <th className="w-28 px-3 py-2 text-right">事件</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-900">
                  {projects.map((p) => (
                    <ProjectRow key={p.id} project={p} checked={selected.has(p.id)} onChange={(checked) => {
                      setSelected((prev) => {
                        const next = new Set(prev);
                        if (checked) next.add(p.id);
                        else next.delete(p.id);
                        return next;
                      });
                    }} />
                  ))}
                  {!preview ? (
                    <tr>
                      <td className="px-3 py-8 text-center text-sm text-zinc-500" colSpan={4}>
                        正在读取本地项目预览...
                      </td>
                    </tr>
                  ) : projects.length === 0 ? (
                    <tr>
                      <td className="px-3 py-8 text-center text-sm text-zinc-500" colSpan={4}>
                        当前没有可迁移的项目。
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <section className="overflow-hidden rounded-lg border border-zinc-900 bg-zinc-950">
          <div className="border-b border-zinc-900 px-4 py-3">
            <div className="text-sm font-semibold">Cloud 目标</div>
            <div className="mt-1 text-xs text-zinc-500">填写云端地址和导入授权信息。</div>
          </div>

          <div className="space-y-4 p-4">
            <label className="field-label">
              Cloud 地址
              <input className="input mt-1" value={cloudURL} onChange={(e) => setCloudURL(e.target.value)} />
            </label>
            <label className="field-label">
              <span className="flex items-center justify-between gap-2">
                <span>导入 Token</span>
                <a className="btn btn-xs btn-outline" href={cloudImportTokensURL(cloudURL)} target="_blank" rel="noreferrer">
                  获取 Token
                </a>
              </span>
              <input className="input mt-1" type="password" value={cloudToken} onChange={(e) => setCloudToken(e.target.value)} />
              <span className="field-hint">在 Cloud 登录后进入“导入自建 logtap”的“导入 Token”，复制 Token。</span>
            </label>
            <label className="field-label">
              组织 ID（可选）
              <input className="input mt-1" value={orgID} onChange={(e) => setOrgID(e.target.value)} placeholder="个人空间留空" />
            </label>

            <div className="rounded-md border border-zinc-900 bg-zinc-950/60 p-3">
              <div className="text-xs font-medium text-zinc-400">迁移摘要</div>
              <div className="mt-3 grid grid-cols-3 gap-2">
                <SummaryStat label="项目" value={selectedProjects.length} />
                <SummaryStat label="日志" value={selectedLogs} />
                <SummaryStat label="事件" value={selectedEvents} />
              </div>
              <div className="mt-3 rounded-md bg-zinc-900/40 px-3 py-2 text-xs leading-5 text-zinc-500">
                Cloud 会为每个导入项目生成新的 project key。
              </div>
            </div>

            <button className="btn btn-md btn-primary w-full" disabled={busy || selected.size === 0} onClick={start}>
              {busy ? "正在创建迁移任务..." : "一键迁移到 Cloud"}
            </button>
          </div>
        </section>
      </div>
    </div>
  );
}

export function CloudMigrationPage() {
  return <CloudMigrationPanel />;
}

function ProjectRow(props: { project: MigrationPreviewProject; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <tr className="text-zinc-200 transition-colors hover:bg-zinc-900/40">
      <td className="px-3 py-2">
        <input className="check-input" type="checkbox" checked={props.checked} onChange={(e) => props.onChange(e.target.checked)} />
      </td>
      <td className="min-w-0 px-3 py-2">
        <div className="truncate font-medium">{props.project.name}</div>
        <div className="truncate font-mono text-xs text-zinc-500">ID {props.project.id}</div>
      </td>
      <td className="px-3 py-2 text-right font-mono text-xs">{formatCount(props.project.logs)}</td>
      <td className="px-3 py-2 text-right font-mono text-xs">{formatCount(props.project.events)}</td>
    </tr>
  );
}

function SummaryStat(props: { label: string; value: number }) {
  return (
    <div className="rounded-md border border-zinc-900 bg-zinc-900/30 px-2.5 py-2">
      <div className="text-[11px] text-zinc-500">{props.label}</div>
      <div className="mt-1 truncate font-mono text-sm text-zinc-100">{formatCount(props.value)}</div>
    </div>
  );
}

function formatCount(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function cloudImportJobURL(cloudURL: string, jobID: string): string {
  const base = cloudURL.replace(/\/+$/, "");
  return `${base}/settings/imports?tab=jobs&job=${encodeURIComponent(jobID)}`;
}

function cloudImportTokensURL(cloudURL: string): string {
  const base = cloudURL.replace(/\/+$/, "");
  return `${base}/settings/imports?tab=tokens`;
}

function buildRange(days: string): { since?: string } {
  if (days === "all") return {};
  const n = Number(days);
  if (!Number.isFinite(n) || n <= 0) return {};
  return { since: new Date(Date.now() - n * 24 * 3600 * 1000).toISOString() };
}
