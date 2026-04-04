import type { FieldProps, SchemaField } from '../types';
import { useFormContext } from '../context/FormContext';
import { getFieldComponent } from './fieldMapper';

interface ObjectFieldProps extends FieldProps {
  schema: SchemaField & { properties?: Record<string, SchemaField> };
  requiredFields?: string[];
}

export function ObjectField(props: ObjectFieldProps) {
  const { path, schema, value, onChange, requiredFields = [], disabled } = props;
  const { errorMap, disabled: contextDisabled } = useFormContext();

  const properties = schema.properties ?? {};
  const entries = Object.entries(properties);
  const isDisabled = disabled || contextDisabled;
  const objectValue = (value as Record<string, unknown>) ?? {};

  const handleChange = (key: string, fieldValue: unknown) => {
    onChange({ ...objectValue, [key]: fieldValue });
  };

  // 如果没有 properties，不渲染任何内容
  if (entries.length === 0) {
    return null;
  }

  return (
    <div className="space-y-3">
      {schema.title && path !== '' && (
        <div className="text-sm font-medium text-zinc-300">
          {schema.title}
          {requiredFields.includes(path) && <span className="ml-1 text-red-400">*</span>}
        </div>
      )}
      {entries.map(([key, fieldSchema]) => {
        const fieldPath = path ? `${path}.${key}` : key;
        const FieldComponent = getFieldComponent(fieldSchema);
        const isRequired = requiredFields.includes(key);
        const fieldValue = objectValue[key] ?? fieldSchema.default;

        return (
          <div key={key}>
            <FieldComponent
              path={fieldPath}
              schema={fieldSchema}
              value={fieldValue}
              onChange={(v) => handleChange(key, v)}
              required={isRequired}
              error={errorMap.get(fieldPath)}
              disabled={isDisabled}
            />
          </div>
        );
      })}
    </div>
  );
}
