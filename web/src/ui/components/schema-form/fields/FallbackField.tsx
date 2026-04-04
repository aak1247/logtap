import { useState } from 'react';
import type { FieldProps } from '../types';

function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function FallbackField(props: FieldProps) {
  const { path, schema, value, onChange, error, disabled } = props;
  const [jsonError, setJsonError] = useState<string | null>(null);

  const handleChange = (text: string) => {
    try {
      const parsed = JSON.parse(text);
      onChange(parsed);
      setJsonError(null);
    } catch {
      setJsonError('无效的 JSON 格式');
      // 不更新 value，保持原始数据
    }
  };

  const textValue = JSON.stringify(value ?? schema.default ?? {}, null, 2);

  return (
    <div>
      <div className="mb-1 text-xs text-zinc-400">
        {schema.title ?? path}
        <span className="ml-2 text-zinc-500">(JSON)</span>
      </div>
      <textarea
        value={textValue}
        onChange={(e) => handleChange(e.target.value)}
        disabled={disabled}
        rows={6}
        spellCheck={false}
        className={cn(
          'w-full rounded-md border px-3 py-2 font-mono text-xs text-zinc-100 outline-none',
          'bg-zinc-950 focus:border-indigo-500',
          error || jsonError ? 'border-red-500' : 'border-zinc-800',
          disabled && 'cursor-not-allowed opacity-60'
        )}
      />
      {(error || jsonError) && (
        <div className="mt-1 text-xs text-red-400">{error || jsonError}</div>
      )}
    </div>
  );
}
