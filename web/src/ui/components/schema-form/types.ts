/** JSON Schema 字段定义（简化版，覆盖插件需求） */
export interface SchemaField {
  type: 'string' | 'integer' | 'number' | 'boolean' | 'array' | 'object';
  title?: string;
  description?: string;
  default?: unknown;
  enum?: (string | number)[];
  format?: string; // 'uri' | 'email' | 'date' 等
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  items?: SchemaField; // array 元素 schema
  properties?: Record<string, SchemaField>; // object 属性
  additionalProperties?: boolean | SchemaField;
  required?: string[]; // 用于嵌套对象
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
  errors?: FieldError[]; // 后端返回的错误
  disabled?: boolean;
  className?: string;
}

/** 单字段组件 Props */
export interface FieldProps {
  path: string; // 字段路径，如 "headers.x-api-key"
  schema: SchemaField;
  value: unknown;
  onChange: (value: unknown) => void;
  required?: boolean;
  error?: string;
  disabled?: boolean;
}
