import type { FieldProps } from '../types';

export function BooleanField(props: FieldProps) {
  const { path, schema, value, onChange, disabled } = props;

  return (
    <label className="flex cursor-pointer items-center justify-between rounded-md border border-zinc-900 bg-zinc-950 px-3 py-2">
      <div className="text-xs text-zinc-300">
        {schema.title ?? path}
      </div>
      <input
        type="checkbox"
        className="toggle toggle-sm"
        checked={Boolean(value)}
        onChange={(e) => onChange(e.target.checked)}
        disabled={disabled}
      />
    </label>
  );
}
