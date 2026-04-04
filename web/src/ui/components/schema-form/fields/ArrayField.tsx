import type { FieldProps } from '../types';
import { FallbackField } from './FallbackField';

function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function ArrayField(props: FieldProps) {
  const { path, schema, value, onChange, disabled } = props;
  const items = Array.isArray(value) ? value : [];
  const itemSchema = schema.items;

  // 如果没有 itemSchema 或者不是简单类型，使用 FallbackField
  if (!itemSchema || !['string', 'integer', 'number'].includes(itemSchema.type)) {
    return <FallbackField {...props} />;
  }

  const getDefaultValue = () => {
    switch (itemSchema.type) {
      case 'string':
        return '';
      case 'integer':
        return 0;
      case 'number':
        return 0;
      default:
        return null;
    }
  };

  const addItem = () => {
    onChange([...items, getDefaultValue()]);
  };

  const updateItem = (index: number, newValue: unknown) => {
    const updated = [...items];
    updated[index] = newValue;
    onChange(updated);
  };

  const removeItem = (index: number) => {
    onChange(items.filter((_, i) => i !== index));
  };

  const handleItemChange = (index: number, rawValue: string) => {
    let parsedValue: unknown;
    if (itemSchema.type === 'integer') {
      const intVal = parseInt(rawValue, 10);
      parsedValue = isNaN(intVal) ? 0 : intVal;
    } else if (itemSchema.type === 'number') {
      const floatVal = parseFloat(rawValue);
      parsedValue = isNaN(floatVal) ? 0 : floatVal;
    } else {
      parsedValue = rawValue;
    }
    updateItem(index, parsedValue);
  };

  return (
    <div>
      <div className="mb-2 text-xs text-zinc-400">
        {schema.title ?? path}
      </div>
      <div className="space-y-2">
        {items.map((item, idx) => (
          <div key={idx} className="flex gap-2">
            <input
              type={itemSchema.type === 'string' ? 'text' : 'number'}
              value={item as string | number}
              onChange={(e) => handleItemChange(idx, e.target.value)}
              disabled={disabled}
              className={cn(
                'flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500',
                disabled && 'cursor-not-allowed opacity-60'
              )}
            />
            <button
              type="button"
              onClick={() => removeItem(idx)}
              disabled={disabled}
              className={cn(
                'px-2 text-zinc-500 hover:text-red-400',
                disabled && 'cursor-not-allowed opacity-60'
              )}
            >
              ×
            </button>
          </div>
        ))}
        <button
          type="button"
          onClick={addItem}
          disabled={disabled}
          className={cn(
            'text-xs text-indigo-400 hover:text-indigo-300',
            disabled && 'cursor-not-allowed opacity-60'
          )}
        >
          + 添加项
        </button>
      </div>
    </div>
  );
}
