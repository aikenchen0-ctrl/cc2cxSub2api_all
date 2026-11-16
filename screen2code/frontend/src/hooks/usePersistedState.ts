import { Dispatch, SetStateAction, useEffect, useState } from 'react';

type PersistedState<T> = [T, Dispatch<SetStateAction<T>>];

function usePersistedState<T>(
  defaultValue: T,
  key: string,
  migrate?: (value: T) => T
): PersistedState<T> {
  const [value, setValue] = useState<T>(() => {
    const stored = window.localStorage.getItem(key);
    if (!stored) return defaultValue;
    const parsed = JSON.parse(stored) as T;
    return migrate ? migrate(parsed) : parsed;
  });

  useEffect(() => {
    window.localStorage.setItem(key, JSON.stringify(value));
  }, [key, value]);

  return [value, setValue];
}

export { usePersistedState };
