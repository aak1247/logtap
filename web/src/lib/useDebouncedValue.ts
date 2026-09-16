import { useEffect, useState } from "react";

// Delays propagating a fast-changing value (e.g. a numeric input) so effects
// that fetch on value change fire once typing settles instead of once per
// keystroke.
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}
