import { useEffect, useMemo, useState } from "react";
import { bootstrap, getSystemStatus } from "../../lib/api";
import { loadSettings, saveSettings } from "../../lib/storage";
import { Panel } from "../components/Panel";
import { useNavigate } from "react-router-dom";

export function BootstrapPage() {
  const initial = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const [apiBase, setApiBase] = useState(initial.apiBase);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [projectName, setProjectName] = useState("Default");
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<"uninitialized" | "running" | "maintenance" | "exception" | "">("");
  const [statusChecked, setStatusChecked] = useState(false);
  const [statusErr, setStatusErr] = useState("");
  const [actionErr, setActionErr] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setStatusErr("");
        setActionErr("");
        setStatusChecked(false);
        const base = apiBase.trim().replace(/\/+$/, "");
        const s = await getSystemStatus(base);
        if (cancelled) return;
        setStatus(s.status);
        setStatusChecked(true);
        if (s.status === "running") nav("/login", { replace: true });
        if (s.status === "maintenance") setStatusErr(s.message || "系统维护中");
        if (s.status === "exception") setStatusErr(s.message || "系统异常");
      } catch (e) {
        if (!cancelled) {
          setStatus("exception");
          setStatusChecked(true);
          setStatusErr(e instanceof Error ? e.message : String(e));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [apiBase, nav]);

  const canBootstrap = statusChecked && status === "uninitialized" && !statusErr;
  const err = actionErr || statusErr;

  return (
    <div className="mx-auto max-w-lg space-y-4">
      <div className="text-lg font-semibold">系统初始化</div>
      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel title="连接">
        <label className="block text-xs text-zinc-400">API Base</label>
        <input
          value={apiBase}
          onChange={(e) => setApiBase(e.target.value)}
          placeholder="http://localhost:8080"
          className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
        />
      </Panel>

      <Panel title="首次使用（创建管理员 + 默认项目）" right={<div className="text-xs text-zinc-500">仅当系统无用户时可用</div>}>
        <div className="grid grid-cols-1 gap-3">
          <div>
            <div className="text-xs text-zinc-400">Email</div>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="admin@example.com"
              className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
            />
          </div>
          <div>
            <div className="text-xs text-zinc-400">Password</div>
            <input
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              type="password"
              placeholder="********"
              className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
            />
          </div>
          <div>
            <div className="text-xs text-zinc-400">默认项目名</div>
            <input
              value={projectName}
              onChange={(e) => setProjectName(e.target.value)}
              placeholder="Default"
              className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
            />
          </div>
          <button
            className="w-full rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
            disabled={busy || !canBootstrap}
            onClick={async () => {
              try {
                setBusy(true);
                setActionErr("");
                const base = apiBase.trim().replace(/\/+$/, "");
                await bootstrap(base, email.trim(), password, projectName.trim() || "Default");
                saveSettings({ apiBase: base, token: "", projectId: "" });
                nav(`/login?email=${encodeURIComponent(email.trim())}`, { replace: true });
              } catch (e) {
                setActionErr(e instanceof Error ? e.message : String(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy ? "初始化中..." : "初始化"}
          </button>
        </div>
      </Panel>
    </div>
  );
}
