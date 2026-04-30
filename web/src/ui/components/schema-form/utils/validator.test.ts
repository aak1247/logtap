import { describe, expect, it } from 'vitest';
import { validateField, validateForm } from './validator';
import type { SchemaField } from '../types';

describe('validateField', () => {
  it('validates required for empty string/undefined/null', () => {
    const schema: SchemaField = { type: 'string' };
    expect(validateField('name', '', schema, true)).toBe('此字段为必填项');
    expect(validateField('name', undefined, schema, true)).toBe('此字段为必填项');
    expect(validateField('name', null, schema, true)).toBe('此字段为必填项');
  });

  it('skips empty non-required value', () => {
    expect(validateField('name', '', { type: 'string' }, false)).toBeNull();
  });

  it('validates string min/max length and uri/email format', () => {
    expect(
      validateField('text', 'ab', { type: 'string', minLength: 3 }, false)
    ).toBe('最少 3 个字符');
    expect(
      validateField('text', 'abcdef', { type: 'string', maxLength: 5 }, false)
    ).toBe('最多 5 个字符');
    expect(
      validateField('url', 'notaurl', { type: 'string', format: 'uri' }, false)
    ).toBe('无效的 URI 格式');
    expect(
      validateField('email', 'bad-email', { type: 'string', format: 'email' }, false)
    ).toBe('无效的邮箱格式');
    expect(
      validateField(
        'email',
        'test@example.com',
        { type: 'string', format: 'email' },
        false
      )
    ).toBeNull();
    expect(validateField('name', 12, { type: 'string' }, false)).toBe('必须为字符串');
  });

  it('validates integer and number ranges and numeric checks', () => {
    expect(
      validateField('port', 1.5, { type: 'integer', minimum: 1, maximum: 10 }, false)
    ).toBe('必须为整数');
    expect(validateField('port', 'abc', { type: 'integer' }, false)).toBe('必须为整数');
    expect(
      validateField('port', '0', { type: 'integer', minimum: 1 }, false)
    ).toBe('最小值为 1');
    expect(
      validateField('port', 11, { type: 'integer', maximum: 10 }, false)
    ).toBe('最大值为 10');
    expect(validateField('score', 'xx', { type: 'number' }, false)).toBe('必须为数字');
    expect(
      validateField('score', '9', { type: 'number', minimum: 10 }, false)
    ).toBe('最小值为 10');
    expect(
      validateField('score', 21, { type: 'number', maximum: 20 }, false)
    ).toBe('最大值为 20');
  });

  it('validates enum membership', () => {
    expect(
      validateField('mode', 'c', { type: 'string', enum: ['a', 'b'] }, false)
    ).toBe('必须为以下值之一: a, b');
    expect(
      validateField('mode', 'a', { type: 'string', enum: ['a', 'b'] }, false)
    ).toBeNull();
  });

  it('validates array and object types', () => {
    expect(validateField('arr', 'x', { type: 'array' }, false)).toBe('必须为数组');
    expect(validateField('obj', [], { type: 'object' }, false)).toBe('必须为对象');
    expect(validateField('arr', [1], { type: 'array' }, false)).toBeNull();
    expect(validateField('obj', { a: 1 }, { type: 'object' }, false)).toBeNull();
  });
});

describe('validateForm', () => {
  it('validates full form including nested object paths', () => {
    const schema = {
      properties: {
        url: { type: 'string' as const, format: 'uri' },
        retries: { type: 'integer' as const, minimum: 1 },
        meta: {
          type: 'object' as const,
          properties: {
            email: { type: 'string' as const, format: 'email' },
          },
        },
      },
      required: ['url'],
    } satisfies { properties: Record<string, SchemaField>; required?: string[] };

    const errors = validateForm(
      {
        url: '',
        retries: 0,
        meta: { email: 'bad' },
      },
      schema
    );

    expect(errors).toEqual([
      { path: 'url', message: '此字段为必填项' },
      { path: 'retries', message: '最小值为 1' },
      { path: 'meta.email', message: '无效的邮箱格式' },
    ]);
  });

  it('returns empty array when all fields are valid', () => {
    const errors = validateForm(
      { url: 'https://example.com', retries: 3 },
      {
        properties: {
          url: { type: 'string', format: 'uri' },
          retries: { type: 'integer', minimum: 1, maximum: 5 },
        },
        required: ['url'],
      }
    );
    expect(errors).toEqual([]);
  });
});

