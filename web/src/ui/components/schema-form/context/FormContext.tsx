import { createContext, useContext, useMemo } from 'react';
import type { FieldError } from '../types';

interface FormContextValue {
  disabled: boolean;
  errorMap: Map<string, string>;
}

const FormContext = createContext<FormContextValue>({
  disabled: false,
  errorMap: new Map(),
});

export function useFormContext() {
  return useContext(FormContext);
}

interface FormProviderProps {
  children: React.ReactNode;
  disabled?: boolean;
  errors?: FieldError[];
}

export function FormProvider({ children, disabled = false, errors = [] }: FormProviderProps) {
  const errorMap = useMemo(() => {
    const map = new Map<string, string>();
    errors.forEach((e) => map.set(e.path, e.message));
    return map;
  }, [errors]);

  const value = useMemo(
    () => ({
      disabled,
      errorMap,
    }),
    [disabled, errorMap]
  );

  return <FormContext.Provider value={value}>{children}</FormContext.Provider>;
}
