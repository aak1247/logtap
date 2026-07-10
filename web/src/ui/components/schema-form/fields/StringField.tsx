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
        <div className="field-label">
          {schema.title ?? path}
          {required && <span className="ml-1 text-red-400">*</span>}
        </div>
        {schema.description && (
          <div className="field-hint mb-1">{schema.description}</div>
        )}
        <input
          type={inputType}
          value={(value as string) ?? ''}
          onChange={(e) => onChange(e.target.value)}
          onBlur={handleBlur}
          disabled={disabled}
          className={cn(
            'input mt-1',
            displayError && 'border-red-500 focus:border-red-500 focus:ring-red-500/20'
          )}
          placeholder={schema.description ?? ''}
        />
      </label>
      {displayError && <div className="mt-1 text-xs text-red-400">{displayError}</div>}
    </div>
  );
}
