import { useState } from 'react';
import type { FieldProps } from '../types';
import { validateField } from '../utils/validator';

function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function StringField(props: FieldProps) {
  const { path, schema, value, onChange, required, error, disabled } = props;
  const [touched, setTouched] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const inputType =
    schema.format === 'uri' ? 'url' : schema.format === 'email' ? 'email' : 'text';

  const handleBlur = () => {
    setTouched(true);
    const err = validateField(path, value, schema, required ?? false);
    setLocalError(err);
  };

  const displayError = touched ? localError ?? error : error;

  return (
    <div>
      <label className="block">
        <div className="text-xs text-zinc-400">
          {schema.title ?? path}
          {required && <span className="ml-1 text-red-400">*</span>}
        </div>
        {schema.description && (
          <div className="mb-1 text-xs text-zinc-500">{schema.description}</div>
        )}
        <input
          type={inputType}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          onBlur={handleBlur}
          disabled={disabled}
          className={cn(
            'mt-1 w-full rounded-md border px-3 py-2 text-sm text-zinc-100 outline-none',
            'bg-zinc-950 focus:border-indigo-500',
            displayError ? 'border-red-500' : 'border-zinc-800',
            disabled && 'cursor-not-allowed opacity-60'
          )}
          placeholder={schema.description ?? ''}
        />
      </label>
      {displayError && <div className="mt-1 text-xs text-red-400">{displayError}</div>}
    </div>
  );
}
