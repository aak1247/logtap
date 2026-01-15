import { useEffect, useMemo, useState } from "react";
import { getSystemStatus, login } from "../../lib/api";
import { loadSettings, saveSettings } from "../../lib/storage";
import { Panel } from "../components/Panel";
import { useLocation, useNavigate } from "react-router-dom";

export function LoginPage() {
  const initial = useMemo(() => loadSettings(), []);
  const nav = useNavigate();
  const loc = useLocation();
  const [apiBase, setApiBase] = useState(initial.apiBase);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [statusErr, setStatusErr] = useState("");
  const [loginErr, setLoginErr] = useState("");
  const [statusChecked, setStatusChecked] = useState(false);

  useEffect(() => {
    if (initial.token) nav("/projects");
  }, [initial.token, nav]);

  useEffect(() => {
    const qp = new URLSearchParams(loc.search);
    const e = qp.get("email");
    if (e) setEmail(e);
  }, [loc.search]);

  useEffect(() => {
    if (initial.token) return;
    let cancelled = false;
    (async () => {
      try {
        setStatusErr("");
        setStatusChecked(false);
        const base = apiBase.trim().replace(/\/+$/, "");
        const s = await getSystemStatus(base);
        if (cancelled) return;
        setStatusChecked(true);
        if (s.status === "uninitialized") nav("/bootstrap", { replace: true });
        if (s.status === "maintenance") setStatusErr(s.message || "系统维护中");
        if (s.status === "exception") setStatusErr(s.message || "系统异常");
      } catch (e) {
        if (!cancelled) {
          setStatusErr(e instanceof Error ? e.message : String(e));
          setStatusChecked(true);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [apiBase, initial.token, nav]);

  const canLogin = statusChecked && !statusErr;
  const err = loginErr || statusErr;

  return (
    <div className="mx-auto max-w-lg space-y-4">
      <div className="text-lg font-semibold">登录</div>
      {err ? (
        <div className="rounded-xl border border-red-900/60 bg-red-950/40 p-4 text-sm text-red-200">
          {err}
        </div>
      ) : null}

      <Panel title="连接">
        <label className="block text-xs text-zinc-400">API Base</label>
        <input
          value={apiBase}
          onChange={(e) => {
            const raw = e.target.value;
            setApiBase(raw);
            const base = raw.trim().replace(/\/+$/, "");
            const s = loadSettings();
            if (s.apiBase !== base) saveSettings({ ...s, apiBase: base });
          }}
          placeholder="http://localhost:8080"
          className="mt-1 w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
        />
      </Panel>

      <Panel title="账号">
        <div className="grid grid-cols-1 gap-3">
          <div>
            <div className="text-xs text-zinc-400">Email</div>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
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
          <div className="flex gap-2">
            <button
              className="flex-1 rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-60"
              disabled={busy || !canLogin}
              onClick={async () => {
                try {
                  setBusy(true);
                  setLoginErr("");
                  const base = apiBase.trim().replace(/\/+$/, "");
                  const res = await login(base, email.trim(), password);
                  saveSettings({ apiBase: base, token: res.token, projectId: "" });
                  window.location.href = "/projects";
                } catch (e) {
                  setLoginErr(e instanceof Error ? e.message : String(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy ? "登录中..." : "登录"}
            </button>
          </div>
        </div>
      </Panel>
    </div>
  );
}
