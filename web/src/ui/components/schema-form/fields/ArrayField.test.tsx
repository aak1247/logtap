import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ArrayField } from './ArrayField';

describe('ArrayField', () => {
  it('adds, updates and removes item for simple array', () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <ArrayField
        path="tags"
        schema={{ type: 'array', title: 'Tags', items: { type: 'string' } }}
        value={[]}
        onChange={onChange}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: '+ 添加项' }));
    expect(onChange).toHaveBeenLastCalledWith(['']);

    rerender(
      <ArrayField
        path="tags"
        schema={{ type: 'array', title: 'Tags', items: { type: 'string' } }}
        value={['a']}
        onChange={onChange}
      />
    );

    const input = screen.getByDisplayValue('a');
    fireEvent.change(input, { target: { value: 'abc' } });
    expect(onChange).toHaveBeenLastCalledWith(['abc']);

    fireEvent.click(screen.getByRole('button', { name: '×' }));
    expect(onChange).toHaveBeenLastCalledWith([]);
  });

  it('falls back to JSON editor for complex item schema', () => {
    render(
      <ArrayField
        path="items"
        schema={{ type: 'array', title: 'Items', items: { type: 'object' } }}
        value={[{ a: 1 }]}
        onChange={() => {}}
      />
    );

    expect(screen.getByText('(JSON)')).toBeInTheDocument();
  });
});

