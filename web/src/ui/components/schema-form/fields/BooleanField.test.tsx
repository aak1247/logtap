import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { BooleanField } from './BooleanField';

describe('BooleanField', () => {
  it('toggles checkbox state', () => {
    const onChange = vi.fn();
    render(
      <BooleanField
        path="enabled"
        schema={{ type: 'boolean', title: 'Enabled' }}
        value={false}
        onChange={onChange}
      />
    );

    const checkbox = screen.getByRole('checkbox');
    fireEvent.click(checkbox);
    expect(onChange).toHaveBeenLastCalledWith(true);
  });
});

