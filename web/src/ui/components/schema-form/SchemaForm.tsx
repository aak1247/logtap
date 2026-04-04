import { useMemo } from 'react';
import type { SchemaFormProps } from './types';
import { FormProvider } from './context/FormContext';
import { ObjectField } from './fields/ObjectField';

function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function SchemaForm(props: SchemaFormProps) {
  const { schema, value, onChange, errors = [], disabled, className } = props;

  // 将 schema 包装成 ObjectField 需要的格式
  const rootSchema = useMemo(
    () => ({
      type: 'object' as const,
      properties: schema.properties,
      title: undefined,
    }),
    [schema.properties]
  );

  // 包装 onChange 以适配 FieldProps 的类型
  const handleChange = (newValue: unknown) => {
    onChange(newValue as Record<string, unknown>);
  };

  return (
    <FormProvider disabled={disabled} errors={errors}>
      <div className={cn('space-y-3', className)}>
        <ObjectField
          path=""
          schema={rootSchema}
          value={value}
          onChange={handleChange}
          requiredFields={schema.required ?? []}
          disabled={disabled}
        />
      </div>
    </FormProvider>
  );
}
