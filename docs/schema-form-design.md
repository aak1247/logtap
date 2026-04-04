# Schema Form 通用插件配置表单系统设计

## 1. 概述

### 目标
将后端 detector 插件的 JSON Schema 自动转换为 React 表单组件，实现新插件零前端开发量。

### 约束
- 无重型 UI 库（不引入 react-jsonschema-form 等）
- 复用现有 Tailwind 暗色风格
- 前端校验 + 后端兜底

---

## 2. 组件架构与文件结构

```
web/src/ui/components/schema-form/
├── index.ts                    # 导出入口
├── SchemaForm.tsx              # 主组件
├── types.ts                    # 类型定义
├── utils/
│   ├── schemaParser.ts         # Schema 解析
│   └── validator.ts            # 前端校验
├── fields/
│   ├── StringField.tsx         # 文本输入
│   ├── NumberField.tsx         # 数字输入（integer/number）
│   ├── BooleanField.tsx        # 开关
│   ├── EnumField.tsx           # 下拉选择
│   ├── ArrayField.tsx          # 数组编辑器
│   ├── ObjectField.tsx         # 嵌套对象
│   └── FallbackField.tsx       # textarea 兜底
└── context/
    └── FormContext.tsx         # 表单状态上下文
```

---

## 3. 类型定义 (types.ts)

```typescript
/** JSON Schema 字段定义（简化版，覆盖插件需求） */
export interface SchemaField {
  type: 'string' | 'integer' | 'number' | 'boolean' | 'array' | 'object';
  title?: string;
  description?: string;
  default?: unknown;
  enum?: (string | number)[];
  format?: string;           // 'uri' | 'email' | 'date' 等
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  items?: SchemaField;       // array 元素 schema
  properties?: Record<string, SchemaField>;  // object 属性
  additionalProperties?: boolean | SchemaField;
}

/** 完整 JSON Schema 结构 */
export interface JsonSchema {
  type: 'object';
  properties: Record<string, SchemaField>;
  required?: string[];
  additionalProperties?: boolean;
}

/** 表单字段错误 */
export interface FieldError {
  path: string;
  message: string;
}

/** SchemaForm Props */
export interface SchemaFormProps {
  schema: JsonSchema;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  errors?: FieldError[];      // 后端返回的错误
  disabled?: boolean;
  className?: string;
}

/** 单字段组件 Props */
export interface FieldProps {
  path: string;               // 字段路径，如 "headers.x-api-key"
  schema: SchemaField;
  value: unknown;
  onChange: (value: unknown) => void;
  required?: boolean;
  error?: string;
  disabled?: boolean;
}
```

---

## 4. SchemaForm 组件 API

### Props

| Prop | 类型 | 必填 | 说明 |
|------|------|------|------|
| `schema` | `JsonSchema` | ✓ | 后端返回的 JSON Schema |
| `value` | `Record<string, unknown>` | ✓ | 表单数据（受控） |
| `onChange` | `(value) => void` | ✓ | 数据变更回调 |
| `errors` | `FieldError[]` | | 后端校验错误，按 path 匹配 |
| `disabled` | `boolean` | | 禁用所有字段 |
| `className` | `string` | | 外层容器样式 |

### 使用示例

```tsx
function MonitorForm({ detectorType }: { detectorType: string }) {
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [schema, setSchema] = useState<JsonSchema | null>(null);
  const [errors, setErrors] = useState<FieldError[]>([]);

  useEffect(() => {
    getDetectorSchema(settings, detectorType).then(res => {
      setSchema(res.schema);
      setConfig(res.defaultConfig ?? {});
    });
  }, [detectorType]);

  const handleSubmit = async () => {
    const res = await saveMonitor({ type: detectorType, config });
    if (res.errors) {
      setErrors(res.errors);  // 后端校验错误回显
    }
  };

  if (!schema) return <div>Loading...</div>;

  return (
    <SchemaForm
      schema={schema}
      value={config}
      onChange={setConfig}
      errors={errors}
    />
  );
}
```

---

## 5. 字段映射规则表

| Schema 类型 | 条件 | 组件 | 备注 |
|-------------|------|------|------|
| `string` | `enum` 存在 | `EnumField` | 下拉选择 |
| `string` | `format: 'uri'` | `StringField` | type=url，前端校验 URI |
| `string` | `format: 'email'` | `StringField` | type=email |
| `string` | 其他 | `StringField` | 普通文本 |
| `integer` | - | `NumberField` | step=1，校验整数 |
| `number` | - | `NumberField` | 浮点数 |
| `boolean` | - | `BooleanField` | Toggle 开关 |
| `array` | `items.type` 基础类型 | `ArrayField` | 简单数组编辑器 |
| `array` | `items` 复杂对象 | `FallbackField` | textarea JSON 编辑 |
| `object` | `properties` 存在 | `ObjectField` | 递归渲染子字段 |
| `object` | 无 `properties` | `FallbackField` | textarea JSON 编辑 |
| 未知/复杂 | - | `FallbackField` | 兜底 |

---

## 6. 校验策略

### 6.1 前端校验（即时反馈）

```typescript
// utils/validator.ts
export function validateField(
  path: string,
  value: unknown,
  schema: SchemaField,
  required: boolean
): string | null {
  // 1. 必填校验
  if (required && (value === undefined || value === null || value === '')) {
    return '此字段为必填项';
  }

  // 2. 类型校验
  if (value !== undefined && value !== null) {
    switch (schema.type) {
      case 'string':
        if (typeof value !== 'string') return '必须为字符串';
        if (schema.minLength && value.length < schema.minLength) {
          return `最少 ${schema.minLength} 个字符`;
        }
        if (schema.maxLength && value.length > schema.maxLength) {
          return `最多 ${schema.maxLength} 个字符`;
        }
        if (schema.format === 'uri' && !isValidUri(value)) {
          return '无效的 URI 格式';
        }
        break;

      case 'integer':
        if (!Number.isInteger(value)) return '必须为整数';
        if (schema.minimum !== undefined && value < schema.minimum) {
          return `最小值为 ${schema.minimum}`;
        }
        if (schema.maximum !== undefined && value > schema.maximum) {
          return `最大值为 ${schema.maximum}`;
        }
        break;

      case 'number':
        if (typeof value !== 'number') return '必须为数字';
        if (schema.minimum !== undefined && value < schema.minimum) {
          return `最小值为 ${schema.minimum}`;
        }
        if (schema.maximum !== undefined && value > schema.maximum) {
          return `最大值为 ${schema.maximum}`;
        }
        break;

      case 'array':
        if (!Array.isArray(value)) return '必须为数组';
        break;

      case 'object':
        if (typeof value !== 'object' || Array.isArray(value)) {
          return '必须为对象';
        }
        break;
    }

    // 3. enum 校验
    if (schema.enum && !schema.enum.includes(value as string | number)) {
      return `必须为以下值之一: ${schema.enum.join(', ')}`;
    }
  }

  return null;
}

function isValidUri(str: string): boolean {
  try {
    new URL(str);
    return true;
  } catch {
    return false;
  }
}
```

### 6.2 校验时机

1. **失焦校验**：字段 blur 时触发
2. **提交校验**：表单提交前全量校验
3. **实时校验**：仅对格式类（URI、email）做实时反馈

### 6.3 后端错误合并

```typescript
function mergeErrors(
  frontendErrors: FieldError[],
  backendErrors: FieldError[]
): FieldError[] {
  const map = new Map<string, string>();
  frontendErrors.forEach(e => map.set(e.path, e.message));
  // 后端错误优先（覆盖前端）
  backendErrors.forEach(e => map.set(e.path, e.message));
  return Array.from(map.entries()).map(([path, message]) => ({ path, message }));
}
```

---

## 7. 集成方案

### 7.1 MonitorTab.tsx 改造

**现状**：
```tsx
// 当前使用 JsonField 组件（textarea）
<JsonField
  label="配置 (JSON)"
  value={form.configJSON}
  onChange={(v) => updateForm('configJSON', v)}
/>
```

**改造后**：
```tsx
import { SchemaForm, parseSchema } from '@/ui/components/schema-form';

function MonitorTab({ settings }: MonitorTabProps) {
  const [form, setForm] = useState<MonitorFormState>({...});
  const [schema, setSchema] = useState<JsonSchema | null>(null);
  const [configObj, setConfigObj] = useState<Record<string, unknown>>({});
  const [errors, setErrors] = useState<FieldError[]>([]);

  // 加载 detector schema
  useEffect(() => {
    if (form.detectorType) {
      loadDetectorSchema(form.detectorType).then(res => {
        setSchema(res.schema);
      });
    }
  }, [form.detectorType]);

  // JSON 字符串 ↔ 对象同步
  useEffect(() => {
    try {
      setConfigObj(JSON.parse(form.configJSON || '{}'));
      setErrors(prev => prev.filter(e => e.path !== '_parse'));
    } catch {
      setErrors([{ path: '_parse', message: '无效的 JSON 格式' }]);
    }
  }, [form.configJSON]);

  const handleConfigChange = (newConfig: Record<string, unknown>) => {
    setConfigObj(newConfig);
    setForm(prev => ({ ...prev, configJSON: JSON.stringify(newConfig, null, 2) }));
  };

  return (
    <div className="space-y-4">
      {/* 其他字段... */}

      {/* 配置区域 */}
      <div>
        <div className="mb-2 text-xs text-zinc-400">插件配置</div>
        {schema ? (
          <SchemaForm
            schema={schema}
            value={configObj}
            onChange={handleConfigChange}
            errors={errors}
            disabled={!form.enabled}
          />
        ) : (
          <div className="text-sm text-zinc-500">
            请先选择检测器类型
          </div>
        )}
      </div>
    </div>
  );
}
```

### 7.2 兼容策略

- 保留 `configJSON` 字段，SchemaForm 修改后同步更新
- 允许用户切换「表单模式」/「JSON 模式」（可选功能）
- 后端 API 无需修改

---

## 8. 关键组件伪代码

### 8.1 SchemaForm.tsx（主组件）

```tsx
import { useFormContext, FormProvider } from './context/FormContext';
import { ObjectField } from './fields/ObjectField';

export function SchemaForm(props: SchemaFormProps) {
  const { schema, value, onChange, errors = [], disabled, className } = props;

  const errorMap = useMemo(() => {
    const map = new Map<string, string>();
    errors.forEach(e => map.set(e.path, e.message));
    return map;
  }, [errors]);

  return (
    <FormProvider value={{ disabled, errorMap }}>
      <div className={cn('space-y-3', className)}>
        <ObjectField
          path=""
          schema={{ type: 'object', properties: schema.properties }}
          value={value}
          onChange={onChange}
          requiredFields={schema.required ?? []}
        />
      </div>
    </FormProvider>
  );
}
```

### 8.2 ObjectField.tsx（对象字段）

```tsx
import { getFieldComponent } from './fieldMapper';

export function ObjectField(props: FieldProps & { requiredFields: string[] }) {
  const { path, schema, value, onChange, requiredFields } = props;
  const { errorMap, disabled } = useFormContext();

  const properties = schema.properties ?? {};
  const entries = Object.entries(properties);

  const handleChange = (key: string, fieldValue: unknown) => {
    onChange({ ...(value as object), [key]: fieldValue });
  };

  return (
    <div className="space-y-3">
      {entries.map(([key, fieldSchema]) => {
        const fieldPath = path ? `${path}.${key}` : key;
        const FieldComponent = getFieldComponent(fieldSchema);
        const isRequired = requiredFields.includes(key);

        return (
          <div key={key}>
            <FieldComponent
              path={fieldPath}
              schema={fieldSchema}
              value={(value as Record<string, unknown>)?.[key] ?? fieldSchema.default}
              onChange={(v) => handleChange(key, v)}
              required={isRequired}
              error={errorMap.get(fieldPath)}
              disabled={disabled}
            />
          </div>
        );
      })}
    </div>
  );
}
```

### 8.3 StringField.tsx（文本字段）

```tsx
export function StringField(props: FieldProps) {
  const { path, schema, value, onChange, required, error, disabled } = props;
  const [touched, setTouched] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const inputType = schema.format === 'uri' ? 'url' :
                    schema.format === 'email' ? 'email' : 'text';

  const handleBlur = () => {
    setTouched(true);
    const err = validateField(path, value, schema, required ?? false);
    setLocalError(err);
  };

  const displayError = touched ? localError : error;

  return (
    <div>
      <label className="block">
        <div className="text-xs text-zinc-400">
          {schema.title ?? path}
          {required && <span className="text-red-400 ml-1">*</span>}
        </div>
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
            disabled && 'opacity-60 cursor-not-allowed'
          )}
          placeholder={schema.description}
        />
      </label>
      {displayError && (
        <div className="mt-1 text-xs text-red-400">{displayError}</div>
      )}
    </div>
  );
}
```

### 8.4 NumberField.tsx（数字字段）

```tsx
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

  return (
    <div>
      <label className="block">
        <div className="text-xs text-zinc-400">
          {schema.title ?? path}
          {required && <span className="text-red-400 ml-1">*</span>}
        </div>
        <div className="text-xs text-zinc-500 mb-1">
          {schema.minimum !== undefined && `最小: ${schema.minimum}`}
          {schema.maximum !== undefined && ` | 最大: ${schema.maximum}`}
        </div>
        <input
          type="number"
          step={isInteger ? 1 : 'any'}
          min={schema.minimum}
          max={schema.maximum}
          value={(value as number) ?? ''}
          onChange={handleChange}
          disabled={disabled}
          className={cn(
            'mt-1 w-full rounded-md border px-3 py-2 text-sm text-zinc-100 outline-none',
            'bg-zinc-950 focus:border-indigo-500',
            error ? 'border-red-500' : 'border-zinc-800',
            disabled && 'opacity-60 cursor-not-allowed'
          )}
        />
      </label>
      {error && <div className="mt-1 text-xs text-red-400">{error}</div>}
    </div>
  );
}
```

### 8.5 EnumField.tsx（枚举/下拉）

```tsx
export function EnumField(props: FieldProps) {
  const { path, schema, value, onChange, required, error, disabled } = props;
  const options = schema.enum ?? [];

  return (
    <div>
      <label className="block">
        <div className="text-xs text-zinc-400">
          {schema.title ?? path}
          {required && <span className="text-red-400 ml-1">*</span>}
        </div>
        <select
          value={(value as string | number) ?? ''}
          onChange={(e) => {
            const v = e.target.value;
            onChange(schema.type === 'integer' ? parseInt(v, 10) : v);
          }}
          disabled={disabled}
          className={cn(
            'mt-1 w-full rounded-md border px-3 py-2 text-sm text-zinc-100 outline-none',
            'bg-zinc-950 focus:border-indigo-500',
            error ? 'border-red-500' : 'border-zinc-800',
            disabled && 'opacity-60 cursor-not-allowed'
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
```

### 8.6 ArrayField.tsx（简单数组）

```tsx
export function ArrayField(props: FieldProps) {
  const { path, schema, value, onChange, disabled } = props;
  const items = (value as unknown[]) ?? [];
  const itemSchema = schema.items;

  const addItem = () => {
    const defaultValue = itemSchema?.type === 'string' ? '' :
                         itemSchema?.type === 'integer' ? 0 : null;
    onChange([...items, defaultValue]);
  };

  const updateItem = (index: number, newValue: unknown) => {
    const updated = [...items];
    updated[index] = newValue;
    onChange(updated);
  };

  const removeItem = (index: number) => {
    onChange(items.filter((_, i) => i !== index));
  };

  // 简单类型数组用 inline 输入
  if (itemSchema?.type === 'string' || itemSchema?.type === 'integer' || itemSchema?.type === 'number') {
    return (
      <div>
        <div className="text-xs text-zinc-400 mb-2">
          {schema.title ?? path}
        </div>
        <div className="space-y-2">
          {items.map((item, idx) => (
            <div key={idx} className="flex gap-2">
              <input
                type={itemSchema.type === 'string' ? 'text' : 'number'}
                value={item as string | number}
                onChange={(e) => updateItem(idx,
                  itemSchema.type === 'string' ? e.target.value :
                  itemSchema.type === 'integer' ? parseInt(e.target.value, 10) :
                  parseFloat(e.target.value)
                )}
                disabled={disabled}
                className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-indigo-500"
              />
              <button
                type="button"
                onClick={() => removeItem(idx)}
                disabled={disabled}
                className="px-2 text-zinc-500 hover:text-red-400"
              >
                ×
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addItem}
            disabled={disabled}
            className="text-xs text-indigo-400 hover:text-indigo-300"
          >
            + 添加项
          </button>
        </div>
      </div>
    );
  }

  // 复杂数组 fallback
  return <FallbackField {...props} />;
}
```

### 8.7 BooleanField.tsx（开关）

```tsx
export function BooleanField(props: FieldProps) {
  const { path, schema, value, onChange, disabled } = props;

  return (
    <label className="flex items-center gap-3 cursor-pointer">
      <input
        type="checkbox"
        checked={value as boolean}
        onChange={(e) => onChange(e.target.checked)}
        disabled={disabled}
        className="toggle toggle-sm toggle-primary"
      />
      <div className="text-xs text-zinc-400">
        {schema.title ?? path}
      </div>
    </label>
  );
}
```

### 8.8 FallbackField.tsx（兜底 textarea）

```tsx
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
      <div className="text-xs text-zinc-400 mb-1">
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
          (error || jsonError) ? 'border-red-500' : 'border-zinc-800',
          disabled && 'opacity-60 cursor-not-allowed'
        )}
      />
      {(error || jsonError) && (
        <div className="mt-1 text-xs text-red-400">{error || jsonError}</div>
      )}
    </div>
  );
}
```

### 8.9 fieldMapper.ts（字段映射器）

```tsx
import { StringField } from './StringField';
import { NumberField } from './NumberField';
import { EnumField } from './EnumField';
import { BooleanField } from './BooleanField';
import { ArrayField } from './ArrayField';
import { ObjectField } from './ObjectField';
import { FallbackField } from './FallbackField';
import type { SchemaField, FieldProps } from '../types';

type FieldComponent = React.FC<FieldProps>;

export function getFieldComponent(schema: SchemaField): FieldComponent {
  // 1. 有 enum 优先
  if (schema.enum) {
    return EnumField;
  }

  // 2. 按类型映射
  switch (schema.type) {
    case 'string':
      return StringField;
    case 'integer':
    case 'number':
      return NumberField;
    case 'boolean':
      return BooleanField;
    case 'array':
      // 简单数组用 ArrayField，复杂用 fallback
      if (schema.items?.type && ['string', 'integer', 'number'].includes(schema.items.type)) {
        return ArrayField;
      }
      return FallbackField;
    case 'object':
      // 有 properties 定义用 ObjectField，否则 fallback
      if (schema.properties && Object.keys(schema.properties).length > 0) {
        return ObjectField;
      }
      return FallbackField;
    default:
      return FallbackField;
  }
}
```

---

## 9. 样式规范

所有字段组件遵循统一的 Tailwind 类名：

```css
/* 容器 */
.field-container: space-y-3

/* 标签 */
.field-label: text-xs text-zinc-400
.field-required: text-red-400 ml-1

/* 输入框 */
.input-base:
  mt-1 w-full rounded-md border px-3 py-2 text-sm text-zinc-100
  bg-zinc-950 outline-none focus:border-indigo-500

/* 状态 */
.input-error: border-red-500
.input-normal: border-zinc-800
.input-disabled: opacity-60 cursor-not-allowed

/* 错误信息 */
.error-text: mt-1 text-xs text-red-400

/* 提示信息 */
.hint-text: text-xs text-zinc-500
```

---

## 10. 扩展性设计

### 10.1 自定义字段组件

支持注册自定义字段渲染器：

```typescript
// 扩展类型
type CustomFieldRenderer = (props: FieldProps) => React.ReactNode;

interface SchemaFormConfig {
  customFields?: {
    [matcher: string]: CustomFieldRenderer;
  };
}

// 使用
<SchemaForm
  schema={schema}
  value={config}
  onChange={setConfig}
  config={{
    customFields: {
      // 按 format 匹配
      'format:markdown': MarkdownEditor,
      // 按 path 匹配
      'path:headers': HeadersEditor,
    }
  }}
/>
```

### 10.2 国际化预留

```typescript
interface SchemaFormI18n {
  required: string;
  invalidFormat: string;
  minValue: string;
  maxValue: string;
  addItem: string;
  removeItem: string;
  selectPlaceholder: string;
}

// 默认中文
const defaultI18n: SchemaFormI18n = {
  required: '此字段为必填项',
  invalidFormat: '格式无效',
  minValue: '最小值为 {min}',
  maxValue: '最大值为 {max}',
  addItem: '+ 添加项',
  removeItem: '删除',
  selectPlaceholder: '请选择...',
};
```

---

## 11. 实现优先级

| 阶段 | 内容 | 预计工时 |
|------|------|----------|
| P0 | 核心组件：SchemaForm, ObjectField, StringField, NumberField, EnumField | 1-2 天 |
| P0 | 校验逻辑 + MonitorTab 集成 | 0.5 天 |
| P1 | ArrayField, BooleanField, FallbackField | 0.5 天 |
| P2 | 错误动画、字段描述展示、暗色主题细节优化 | 0.5 天 |
| P3 | 自定义字段注册、国际化支持 | 按需 |

---

## 12. 测试策略

### 12.1 单元测试

```typescript
// validator.test.ts
describe('validateField', () => {
  it('should validate required field', () => {
    expect(validateField('url', '', { type: 'string' }, true))
      .toBe('此字段为必填项');
  });

  it('should validate URI format', () => {
    expect(validateField('url', 'invalid', { type: 'string', format: 'uri' }, false))
      .toBe('无效的 URI 格式');
    expect(validateField('url', 'https://example.com', { type: 'string', format: 'uri' }, false))
      .toBeNull();
  });

  it('should validate integer range', () => {
    expect(validateField('port', 0, { type: 'integer', minimum: 1, maximum: 65535 }, false))
      .toBe('最小值为 1');
    expect(validateField('port', 70000, { type: 'integer', minimum: 1, maximum: 65535 }, false))
      .toBe('最大值为 65535');
  });
});
```

### 12.2 集成测试

使用各插件的 schema 做端到端测试：

```typescript
const testCases = [
  {
    plugin: 'http_check',
    config: { url: 'https://example.com', method: 'GET' },
    expectedValid: true,
  },
  {
    plugin: 'tcp_check',
    config: { host: 'localhost', port: 8080 },
    expectedValid: true,
  },
  {
    plugin: 'metric_threshold',
    config: { field: 'cpu', op: 'gt', value: 80 },
    expectedValid: true,
  },
];
```

---

## 13. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Schema 变更导致表单异常 | 中 | 版本兼容检测 + Fallback 兜底 |
| 复杂嵌套结构渲染性能 | 低 | 虚拟化长列表、懒加载深层对象 |
| 后端校验与前端不一致 | 中 | 共享校验规则定义，后端返回标准化错误格式 |

---

## 附录：现有插件 Schema 汇总

### http_check
```json
{
  "type": "object",
  "properties": {
    "url": {"type": "string", "format": "uri"},
    "method": {"type": "string"},
    "headers": {"type": "object", "additionalProperties": {"type": "string"}},
    "body": {"type": "string"},
    "expectStatus": {"type": "array", "items": {"type": "integer"}},
    "expectBodySubstring": {"type": "string"},
    "timeoutMs": {"type": "integer", "minimum": 100, "maximum": 60000},
    "minTlsValidDays": {"type": "integer", "minimum": 0, "maximum": 3650}
  },
  "required": ["url"]
}
```

### tcp_check
```json
{
  "type": "object",
  "properties": {
    "host": {"type": "string"},
    "port": {"type": "integer", "minimum": 1, "maximum": 65535},
    "timeoutMs": {"type": "integer", "minimum": 100, "maximum": 60000}
  },
  "required": ["host", "port"]
}
```

### metric_threshold
```json
{
  "type": "object",
  "properties": {
    "field": {"type": "string"},
    "op": {"type": "string", "enum": [">", "<", ">=", "<=", "between"]},
    "value": {"type": "number"},
    "min": {"type": "number"},
    "max": {"type": "number"},
    "severityOnViolation": {"type": "string"}
  },
  "required": ["field", "op"]
}
```

### log_basic（假设）
```json
{
  "type": "object",
  "properties": {
    "pattern": {"type": "string"},
    "level": {"type": "string", "enum": ["error", "warn", "info", "debug"]},
    "threshold": {"type": "integer", "minimum": 1},
    "windowSeconds": {"type": "integer", "minimum": 1, "maximum": 3600}
  },
  "required": ["pattern", "level"]
}
```
