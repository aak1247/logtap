import { useEffect, useMemo, useState } from "react";
import {
  createProject,
  listProjects,
  type Project,
} from "../../lib/api";
import { loadSettings, saveSettings } from "../../lib/storage";
import { Panel } from "../components/Panel";
import { useNavigate } from "react-router-dom";

export function ProjectsPage() {
  const settings = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [projects, setProjects] = useState<Project[]>([]);
  const [newProjectName, setNewProjectName] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [activeProjectId, setActiveProjectId] = useState(
    () => loadSettings().projectId,
  );

  useEffect(() => {
    if (!settings.token) {
      nav("/login");
    }
  }, [settings.token, nav]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setErr("");
        const res = await listProjects(settings);
        if (cancelled) return;
        setProjects(res.items);
      } catch (e) {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [settings.apiBase, settings.token]);

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between">
        <div>
          <div className="text-lg font-semibold">切换项目</div>
          <div className="mt-1 text-sm text-zinc-400">选择项目后进入系统</div>
        </div>
      </div>

      {!activeProjectId ? (
        <div className="rounded-xl border border-zinc-900 bg-zinc-950/60 p-4 text-sm text-zinc-300">
          未选择项目，请先选择一个项目进入系统。
        </div>
      ) : null}

      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel
        title="项目列表"
        right={
          <button
            className="rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
            disabled={busy}
            onClick={async () => {
              try {
                setBusy(true);
                const p = await createProject(settings, newProjectName.trim());
                setNewProjectName("");
                setProjects((prev) => [...prev, p]);
              } catch (e) {
                setErr(e instanceof Error ? e.message : String(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            新建
          </button>
        }
      >
        <div className="space-y-3">
          <div>
            <div className="text-xs text-zinc-400">新项目名称</div>
            <input
              value={newProjectName}
              onChange={(e) => setNewProjectName(e.target.value)}
              placeholder="My Project"
              className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
            />
          </div>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            {projects.map((p) => {
              const active = activeProjectId === String(p.id);
              return (
                <div
                  key={p.id}
                  className={`rounded-lg border border-zinc-900 bg-zinc-950 p-4 ${
                    active ? "ring-1 ring-indigo-500/40" : ""
                  }`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="text-sm font-semibold text-zinc-100">{p.name}</div>
                      <div className="mt-1 text-xs text-zinc-500">ID {p.id}</div>
                    </div>
                    <button
                      className="rounded-md border border-zinc-800 bg-zinc-950 p-1.5 text-zinc-300 hover:bg-zinc-900 hover:text-zinc-100"
                      aria-label="项目设置"
                      onClick={() => {
                        saveSettings({ ...settings, projectId: String(p.id) });
                        setActiveProjectId(String(p.id));
                        nav(`/projects?settings=project&projectId=${p.id}`);
                      }}
                    >
                      <SettingsIcon className="h-4 w-4" />
                    </button>
                  </div>
                  <div className="mt-3 flex items-center gap-2">
                    <button
                      className="rounded-md bg-indigo-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-indigo-500"
                      onClick={() => {
                        saveSettings({ ...settings, projectId: String(p.id) });
                        setActiveProjectId(String(p.id));
                        nav("/");
                      }}
                    >
                      进入
                    </button>
                    {active ? (
                      <span className="text-xs text-emerald-400">当前项目</span>
                    ) : null}
                  </div>
                </div>
              );
            })}
            {projects.length === 0 ? (
              <div className="rounded-lg border border-dashed border-zinc-800 p-4 text-sm text-zinc-500">
                暂无项目
              </div>
            ) : null}
          </div>
        </div>
      </Panel>
    </div>
  );
}

function SettingsIcon(props: { className?: string }) {
  return (
    <svg
      className={props.className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M4 6h16M4 12h16M4 18h16" />
      <circle cx="8" cy="6" r="2" />
      <circle cx="16" cy="12" r="2" />
      <circle cx="10" cy="18" r="2" />
    </svg>
  );
}
