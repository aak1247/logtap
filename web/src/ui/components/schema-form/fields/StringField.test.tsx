import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { StringField } from './StringField';

describe('StringField', () => {
  it('handles input and shows blur validation error', () => {
    const onChange = vi.fn();
    render(
      <StringField
        path="url"
        schema={{ type: 'string', title: 'URL', format: 'uri' }}
        value=""
        onChange={onChange}
        required
      />
    );

    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'https://example.com' } });
    expect(onChange).toHaveBeenLastCalledWith('https://example.com');

    fireEvent.change(input, { target: { value: '' } });
    fireEvent.blur(input);
    expect(screen.getByText('此字段为必填项')).toBeInTheDocument();
  });

  it('shows backend error when provided', () => {
    render(
      <StringField
        path="name"
        schema={{ type: 'string', title: 'Name' }}
        value=""
        onChange={() => {}}
        error="后端错误"
      />
    );

    expect(screen.getByText('后端错误')).toBeInTheDocument();
  });
});
