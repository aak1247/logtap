import type { SchemaField } from '../types';

/**
 * 校验单个字段
 */
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

  // 如果值为空且非必填，跳过其他校验
  if (value === undefined || value === null || value === '') {
    return null;
  }

  // 2. 类型校验
  switch (schema.type) {
    case 'string':
      if (typeof value !== 'string') return '必须为字符串';
      if (schema.minLength !== undefined && value.length < schema.minLength) {
        return `最少 ${schema.minLength} 个字符`;
      }
      if (schema.maxLength !== undefined && value.length > schema.maxLength) {
        return `最多 ${schema.maxLength} 个字符`;
      }
      if (schema.format === 'uri' && !isValidUri(value)) {
        return '无效的 URI 格式';
      }
      if (schema.format === 'email' && !isValidEmail(value)) {
        return '无效的邮箱格式';
      }
      break;

    case 'integer':
      if (!Number.isInteger(value)) {
        if (typeof value === 'string') {
          const num = parseInt(value, 10);
          if (isNaN(num)) return '必须为整数';
        } else {
          return '必须为整数';
        }
      }
      const intVal = typeof value === 'number' ? value : parseInt(String(value), 10);
      if (schema.minimum !== undefined && intVal < schema.minimum) {
        return `最小值为 ${schema.minimum}`;
      }
      if (schema.maximum !== undefined && intVal > schema.maximum) {
        return `最大值为 ${schema.maximum}`;
      }
      break;

    case 'number':
      if (typeof value !== 'number') {
        if (typeof value === 'string') {
          const num = parseFloat(value);
          if (isNaN(num)) return '必须为数字';
        } else {
          return '必须为数字';
        }
      }
      const numVal = typeof value === 'number' ? value : parseFloat(String(value));
      if (schema.minimum !== undefined && numVal < schema.minimum) {
        return `最小值为 ${schema.minimum}`;
      }
      if (schema.maximum !== undefined && numVal > schema.maximum) {
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

  return null;
}

/**
 * 校验整个表单
 */
export function validateForm(
  value: Record<string, unknown>,
  schema: { properties: Record<string, SchemaField>; required?: string[] }
): Array<{ path: string; message: string }> {
  const errors: Array<{ path: string; message: string }> = [];
  const requiredFields = schema.required ?? [];

  for (const [key, fieldSchema] of Object.entries(schema.properties)) {
    const fieldValue = value[key];
    const isRequired = requiredFields.includes(key);
    const error = validateField(key, fieldValue, fieldSchema, isRequired);
    if (error) {
      errors.push({ path: key, message: error });
    }

    // 递归校验嵌套对象
    if (fieldSchema.type === 'object' && fieldSchema.properties && typeof fieldValue === 'object' && fieldValue !== null && !Array.isArray(fieldValue)) {
      const nestedErrors = validateForm(fieldValue as Record<string, unknown>, {
        properties: fieldSchema.properties,
        required: [],
      });
      for (const nestedError of nestedErrors) {
        errors.push({
          path: `${key}.${nestedError.path}`,
          message: nestedError.message,
        });
      }
    }
  }

  return errors;
}

function isValidUri(str: string): boolean {
  try {
    new URL(str);
    return true;
  } catch {
    return false;
  }
}

function isValidEmail(str: string): boolean {
  // 简单的邮箱校验
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailRegex.test(str);
}
