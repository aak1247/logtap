export function Sparkline(props: {
  values: number[];
  width?: number;
  height?: number;
}) {
  const width = props.width ?? 320;
  const height = props.height ?? 80;
  const values = props.values.length ? props.values : [0];
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const range = Math.max(max - min, 1);

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

