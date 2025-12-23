export function StatCard(props: {
  title: string;
  value: string | number;
  hint?: string;
}) {
  return (
    <div className="rounded-xl border border-zinc-900 bg-zinc-950 p-4">
      <div className="text-xs text-zinc-400">{props.title}</div>
      <div className="mt-1 text-2xl font-semibold">{props.value}</div>
      {props.hint ? (
        <div className="mt-2 text-xs text-zinc-500">{props.hint}</div>
      ) : null}
    </div>
  );
}

