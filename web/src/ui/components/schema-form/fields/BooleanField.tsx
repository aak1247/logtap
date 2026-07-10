import type { FieldProps } from '../types';

export function BooleanField(props: FieldProps) {
  const { path, schema, value, onChange, disabled } = props;

  return (
    <label className="flex cursor-pointer items-start justify-between gap-3 rounded-md border border-zinc-900 bg-zinc-950 px-3 py-2 transition-colors hover:border-zinc-800 hover:bg-zinc-900/30">
      <div>
        <div className="field-label">{schema.title ?? path}</div>
        {schema.description ? <div className="field-hint">{schema.description}</div> : null}
      </div>
      <input
        type="checkbox"
        className="check-input mt-0.5"
        checked={Boolean(value)}
        onChange={(e) => onChange(e.target.checked)}
        disabled={disabled}
      />
    </label>
  );
}
