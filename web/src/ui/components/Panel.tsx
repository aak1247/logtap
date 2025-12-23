import type { ReactNode } from "react";

export function Panel(props: {
  title: string;
  right?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="rounded-xl border border-zinc-900 bg-zinc-950">
      <header className="flex items-center justify-between border-b border-zinc-900 px-4 py-3">
        <div className="text-sm font-semibold">{props.title}</div>
        {props.right ? <div>{props.right}</div> : null}
      </header>
      <div className="p-4">{props.children}</div>
    </section>
  );
}

