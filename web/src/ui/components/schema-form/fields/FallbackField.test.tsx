import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FallbackField } from './FallbackField';

describe('FallbackField', () => {
  it('edits valid JSON and calls onChange with parsed object', () => {
    const onChange = vi.fn();
    render(
      <FallbackField
        path="payload"
        schema={{ type: 'object', title: 'Payload' }}
        value={{ a: 1 }}
        onChange={onChange}
      />
    );

    const textarea = screen.getByRole('textbox');
    fireEvent.change(textarea, { target: { value: '{"a":2}' } });
    expect(onChange).toHaveBeenLastCalledWith({ a: 2 });
  });

  it('shows JSON error and does not call onChange on invalid input', () => {
    const onChange = vi.fn();
    render(
      <FallbackField
        path="payload"
        schema={{ type: 'object', title: 'Payload' }}
        value={{}}
        onChange={onChange}
      />
    );

    fireEvent.change(screen.getByRole('textbox'), { target: { value: '{bad json' } });
    expect(screen.getByText('无效的 JSON 格式')).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });
});

