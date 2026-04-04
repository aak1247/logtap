import type { FieldProps } from '../types';

function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function EnumField(props: FieldProps) {
  const { path, schema, value, onChange, required, error, disabled } = props;
  const options = schema.enum ?? [];

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const raw = e.target.value;
    if (raw === '') {
      onChange(undefined);
      return;
    }
    // 根据 schema 类型转换
    if (schema.type === 'integer' || schema.type === 'number') {
      onChange(parseFloat(raw));
    } else {
      onChange(raw);
    }
  };

  const displayValue = value !== undefined && value !== null ? String(value) : '';

  return (
    <div>
      <label className="block">
        <div className="text-xs text-zinc-400">
          {schema.title ?? path}
          {required && <span className="ml-1 text-red-400">*</span>}
        </div>
        <select
          value={displayValue}
          onChange={handleChange}
          disabled={disabled}
          className={cn(
            'mt-1 w-full rounded-md border px-3 py-2 text-sm text-zinc-100 outline-none',
            'bg-zinc-950 focus:border-indigo-500',
            error ? 'border-red-500' : 'border-zinc-800',
            disabled && 'cursor-not-allowed opacity-60'
          )}
        >
          <option value="">请选择...</option>
          {options.map((opt) => (
            <option key={String(opt)} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      </label>
      {error && <div className="mt-1 text-xs text-red-400">{error}</div>}
    </div>
  );
}
