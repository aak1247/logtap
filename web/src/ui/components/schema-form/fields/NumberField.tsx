import type { FieldProps } from '../types';

function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function NumberField(props: FieldProps) {
  const { path, schema, value, onChange, required, error, disabled } = props;
  const isInteger = schema.type === 'integer';

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value;
    if (raw === '') {
      onChange(undefined);
      return;
    }
    const num = isInteger ? parseInt(raw, 10) : parseFloat(raw);
    if (!isNaN(num)) {
      onChange(num);
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
        {(schema.minimum !== undefined || schema.maximum !== undefined) && (
          <div className="mb-1 text-xs text-zinc-500">
            {schema.minimum !== undefined && `最小: ${schema.minimum}`}
            {schema.minimum !== undefined && schema.maximum !== undefined && ' | '}
            {schema.maximum !== undefined && `最大: ${schema.maximum}`}
          </div>
        )}
        <input
          type="number"
          step={isInteger ? 1 : 'any'}
          min={schema.minimum}
          max={schema.maximum}
          value={displayValue}
          onChange={handleChange}
          disabled={disabled}
          className={cn(
            'mt-1 w-full rounded-md border px-3 py-2 text-sm text-zinc-100 outline-none',
            'bg-zinc-950 focus:border-indigo-500',
            error ? 'border-red-500' : 'border-zinc-800',
            disabled && 'cursor-not-allowed opacity-60'
          )}
        />
      </label>
      {error && <div className="mt-1 text-xs text-red-400">{error}</div>}
    </div>
  );
}
