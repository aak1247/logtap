import type { SchemaField, FieldProps } from '../types';
import { StringField } from './StringField';
import { NumberField } from './NumberField';
import { EnumField } from './EnumField';
import { BooleanField } from './BooleanField';
import { ArrayField } from './ArrayField';
import { ObjectField } from './ObjectField';
import { FallbackField } from './FallbackField';

type FieldComponent = React.FC<FieldProps>;

export function getFieldComponent(schema: SchemaField): FieldComponent {
  // 1. 有 enum 优先
  if (schema.enum && schema.enum.length > 0) {
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
      if (
        schema.items?.type &&
        ['string', 'integer', 'number'].includes(schema.items.type)
      ) {
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
