import { useEffect, useMemo } from "react";
import { Link, NavLink, useParams } from "react-router-dom";
import { Markdown } from "../components/Markdown";
import { findDoc, groupedDocs } from "../docs/registry";

export function DocsPage() {
  const params = useParams();
  const id = (params["*"] ?? "").replace(/^\/+/, "") || "overview";
  const doc = useMemo(() => findDoc(id), [id]);

  useEffect(() => {
    window.scrollTo({ top: 0 });
  }, [doc.id]);

  const groups = groupedDocs();

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-20 border-b border-zinc-900 bg-zinc-950/90 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3">
          <div className="flex items-center gap-3">
            <div className="h-8 w-8 rounded-lg bg-gradient-to-br from-indigo-500 to-cyan-500" />
            <div className="flex flex-col leading-tight">
              <div className="text-sm font-semibold">logtap 文档</div>
              <div className="text-xs text-zinc-400">部署与集成指引</div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Link
              to="/"
              className="rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-900"
            >
              返回控制台
            </Link>
          </div>
        </div>
      </header>

      <main className="mx-auto grid max-w-6xl grid-cols-1 gap-6 px-4 py-6 md:grid-cols-[16rem_1fr]">
        <aside className="md:sticky md:top-[4.5rem] md:max-h-[calc(100vh-5rem)] md:overflow-auto">
          <div className="rounded-xl border border-zinc-900 bg-zinc-950/40 p-3">
            {groups.map(([group, items]) => (
              <div key={group} className="mb-4 last:mb-0">
                <div className="px-2 py-1 text-xs font-semibold text-zinc-400">
                  {group}
                </div>
                <div className="mt-1 flex flex-col gap-1">
                  {items.map((it) => (
                    <NavLink
                      key={it.id}
                      to={`/docs/${it.id}`}
                      className={({ isActive }) =>
                        `rounded-md px-2 py-1.5 text-sm hover:bg-zinc-900 ${
                          isActive
                            ? "bg-zinc-900 text-zinc-100"
                            : "text-zinc-300"
                        }`
                      }
                      end
                    >
                      {it.title}
                    </NavLink>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </aside>

        <section className="rounded-xl border border-zinc-900 bg-zinc-950/40 p-5">
          <Markdown content={doc.content} />
        </section>
      </main>
    </div>
  );
}
