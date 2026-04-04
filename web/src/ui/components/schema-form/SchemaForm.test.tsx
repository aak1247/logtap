import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { SchemaForm } from './SchemaForm';
import type { JsonSchema } from './types';

describe('SchemaForm', () => {
  const schema: JsonSchema = {
    type: 'object',
    properties: {
      name: { type: 'string', title: 'Name' },
      level: { type: 'integer', title: 'Level' },
      mode: { type: 'string', title: 'Mode', enum: ['on', 'off'] },
    },
    required: ['name'],
  };

  it('renders fields based on schema and updates values', () => {
    const onChange = vi.fn();
    render(<SchemaForm schema={schema} value={{}} onChange={onChange} />);

    const nameInput = screen.getByRole('textbox');
    const levelInput = screen.getByRole('spinbutton');
    const modeSelect = screen.getByRole('combobox');

    expect(nameInput).toBeInTheDocument();
    expect(levelInput).toBeInTheDocument();
    expect(modeSelect).toBeInTheDocument();

    fireEvent.change(nameInput, { target: { value: 'abc' } });
    expect(onChange).toHaveBeenLastCalledWith({ name: 'abc' });
  });

  it('passes errors down and shows error text', () => {
    render(
      <SchemaForm
        schema={schema}
        value={{}}
        onChange={() => {}}
        errors={[{ path: 'name', message: '后端错误' }]}
      />
    );

    expect(screen.getByText('后端错误')).toBeInTheDocument();
  });

  it('disables all rendered fields when disabled is true', () => {
    render(<SchemaForm schema={schema} value={{}} onChange={() => {}} disabled />);

    expect(screen.getByRole('textbox')).toBeDisabled();
    expect(screen.getByRole('spinbutton')).toBeDisabled();
    expect(screen.getByRole('combobox')).toBeDisabled();
  });
});
