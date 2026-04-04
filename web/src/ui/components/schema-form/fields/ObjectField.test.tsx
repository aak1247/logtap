import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ObjectField } from './ObjectField';
import { FormProvider } from '../context/FormContext';

describe('ObjectField', () => {
  it('renders nested fields and updates object value', () => {
    const onChange = vi.fn();
    render(
      <FormProvider>
        <ObjectField
          path=""
          schema={{
            type: 'object',
            properties: {
              name: { type: 'string', title: 'Name' },
              nested: {
                type: 'object',
                title: 'Nested',
                properties: {
                  count: { type: 'integer', title: 'Count' },
                },
              },
            },
          }}
          value={{ name: 'a', nested: { count: 1 } }}
          onChange={onChange}
        />
      </FormProvider>
    );

    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'new' } });
    expect(onChange).toHaveBeenLastCalledWith({ name: 'new', nested: { count: 1 } });

    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '2' } });
    expect(onChange).toHaveBeenLastCalledWith({ name: 'a', nested: { count: 2 } });
  });

  it('returns null when object schema has no properties', () => {
    const { container } = render(
      <FormProvider>
        <ObjectField
          path=""
          schema={{ type: 'object', properties: {} }}
          value={{}}
          onChange={() => {}}
        />
      </FormProvider>
    );

    expect(container).toBeEmptyDOMElement();
  });
});
