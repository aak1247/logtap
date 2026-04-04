import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { EnumField } from './EnumField';

describe('EnumField', () => {
  it('handles dropdown selection for string enum', () => {
    const onChange = vi.fn();
    render(
      <EnumField
        path="mode"
        schema={{ type: 'string', title: 'Mode', enum: ['on', 'off'] }}
        value={undefined}
        onChange={onChange}
      />
    );

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'off' } });
    expect(onChange).toHaveBeenLastCalledWith('off');
  });

  it('parses number enum values', () => {
    const onChange = vi.fn();
    render(
      <EnumField
        path="level"
        schema={{ type: 'number', title: 'Level', enum: [1, 2] }}
        value={undefined}
        onChange={onChange}
      />
    );

    fireEvent.change(screen.getByRole('combobox'), { target: { value: '2' } });
    expect(onChange).toHaveBeenLastCalledWith(2);
  });
});
