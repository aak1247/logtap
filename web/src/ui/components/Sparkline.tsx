export function Sparkline(props: {
  values: number[];
  width?: number;
  height?: number;
  variant?: "line" | "column";
}) {
  const width = props.width ?? 320;
  const height = props.height ?? 80;
  const values = props.values.length ? props.values : [0];
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const range = Math.max(max - min, 1);
  const variant = props.variant ?? "line";

  if (variant === "column") {
    const n = values.length;
    const barGap = 1; // gap between bars
    const barWidth = n > 0 ? Math.max(1, (width - (n + 1) * barGap) / n) : width - 2;
    const bars = values.map((v, i) => {
      const x = barGap + i * (barWidth + barGap);
      const h = ((v - min) / range) * (height - 2);
      const y = height - 1 - h;
      return { x, y, w: barWidth, h };
    });

    return (
      <svg width={width} height={height} className="block">
        {bars.map((b, idx) => (
          <rect
            key={idx}
            x={b.x}
            y={b.y}
            width={b.w}
            height={b.h}
            rx={1}
            className="fill-indigo-400/80"
          />
        ))}
      </svg>
    );
  }

  const points = values
    .map((v, i) => {
      const x = values.length === 1 ? 0 : (i / (values.length - 1)) * (width - 2) + 1;
      const y = height - 1 - ((v - min) / range) * (height - 2);
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");

  return (
    <svg width={width} height={height} className="block">
      <polyline
        points={points}
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        className="text-indigo-400"
      />
    </svg>
  );
}

