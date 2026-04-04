import { describe, expect, it } from 'vitest';
import { getFieldComponent } from './fieldMapper';
import { StringField } from './StringField';
import { NumberField } from './NumberField';
import { EnumField } from './EnumField';
import { BooleanField } from './BooleanField';
import { ArrayField } from './ArrayField';
import { ObjectField } from './ObjectField';
import { FallbackField } from './FallbackField';

describe('getFieldComponent', () => {
  it('maps primitive schema types to the expected components', () => {
    expect(getFieldComponent({ type: 'string' })).toBe(StringField);
    expect(getFieldComponent({ type: 'integer' })).toBe(NumberField);
    expect(getFieldComponent({ type: 'number' })).toBe(NumberField);
    expect(getFieldComponent({ type: 'boolean' })).toBe(BooleanField);
  });

  it('prioritizes enum mapping to EnumField', () => {
    expect(
      getFieldComponent({ type: 'string', enum: ['a', 'b'] })
    ).toBe(EnumField);
    expect(
      getFieldComponent({ type: 'number', enum: [1, 2] })
    ).toBe(EnumField);
  });

  it('maps arrays by item complexity', () => {
    expect(
      getFieldComponent({ type: 'array', items: { type: 'string' } })
    ).toBe(ArrayField);
    expect(
      getFieldComponent({ type: 'array', items: { type: 'integer' } })
    ).toBe(ArrayField);
    expect(
      getFieldComponent({ type: 'array', items: { type: 'object' } })
    ).toBe(FallbackField);
    expect(getFieldComponent({ type: 'array' })).toBe(FallbackField);
  });

  it('maps object based on properties existence', () => {
    expect(
      getFieldComponent({
        type: 'object',
        properties: { name: { type: 'string' } },
      })
    ).toBe(ObjectField);
    expect(getFieldComponent({ type: 'object', properties: {} })).toBe(FallbackField);
    expect(getFieldComponent({ type: 'object' })).toBe(FallbackField);
  });

  it('falls back for unknown types', () => {
    expect(getFieldComponent({ type: 'unknown' as never })).toBe(FallbackField);
  });
});

