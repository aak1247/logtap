import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { NumberField } from './NumberField';

describe('NumberField', () => {
  it('handles numeric input and integer step', () => {
    const onChange = vi.fn();
    render(
      <NumberField
        path="port"
        schema={{ type: 'integer', title: 'Port', minimum: 1, maximum: 10 }}
        value={1}
        onChange={onChange}
      />
    );

    const input = screen.getByRole('spinbutton');
    expect(input).toHaveAttribute('step', '1');
    fireEvent.change(input, { target: { value: '3' } });
    expect(onChange).toHaveBeenLastCalledWith(3);
    fireEvent.change(input, { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });
});
