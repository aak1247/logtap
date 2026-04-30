import { useState } from "react";
import { useNavigate } from "react-router-dom";
import type { WidgetProps } from "../widgets/registry";

export function SearchWidget(props: WidgetProps) {
  const [q, setQ] = useState("");
  const nav = useNavigate();

  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-950 p-4">
      <div className="text-sm font-medium text-zinc-300">搜索</div>
      <form
        className="mt-2 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          nav(`/logs?q=${encodeURIComponent(q)}`);
        }}
      >
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="level:error tag:api message:timeout"
          className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
        />
        <button type="submit" className="btn btn-md btn-primary">
          搜索
        </button>
      </form>
    </div>
  );
}
