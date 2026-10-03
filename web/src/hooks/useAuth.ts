import { useState, useCallback, useEffect } from 'react';
import type { User, AuthSource } from '../types';

const STORAGE_KEY_USER = 'redwolf_user';
const STORAGE_KEY_TOKEN = 'redwolf_token';

export function useAuth() {
  const [user, setUser] = useState<User | null>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY_USER);
      if (stored) {
        return JSON.parse(stored);
      }
    } catch {
      // Fallback
    }
    // Default to null so user sees the login gate
    return null;
  });

  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // Validate existing session token against RedWolf Core on mount
  useEffect(() => {
    const token = localStorage.getItem(STORAGE_KEY_TOKEN);
    if (!token) return;

    fetch('/api/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => {
        if (res.ok) {
          return res.json();
        }
        if (res.status === 401 || res.status === 403) {
          localStorage.removeItem(STORAGE_KEY_TOKEN);
          localStorage.removeItem(STORAGE_KEY_USER);
          setUser(null);
        }
        return null;
      })
      .then((validatedUser) => {
        if (validatedUser) {
          setUser(validatedUser);
          localStorage.setItem(STORAGE_KEY_USER, JSON.stringify(validatedUser));
        }
      })
      .catch(() => {
        // Retain current session in offline/standalone client
      });
  }, []);

  const login = useCallback(async (username: string, password: string, source: AuthSource = 'LOCAL') => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password, source }),
      });

      if (res.ok) {
        const data = await res.json();
        setUser(data.user);
        localStorage.setItem(STORAGE_KEY_TOKEN, data.token);
        localStorage.setItem(STORAGE_KEY_USER, JSON.stringify(data.user));
        return data.user;
      }

      const errData = await res.json().catch(() => ({ error: 'Invalid credentials or directory unreachable' }));
      throw new Error(errData.error || 'Authentication failed');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Authentication failed';
      setError(msg);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  const logout = useCallback(async () => {
    const token = localStorage.getItem(STORAGE_KEY_TOKEN);
    if (token) {
      fetch('/api/auth/logout', {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {});
    }
    localStorage.removeItem(STORAGE_KEY_TOKEN);
    localStorage.removeItem(STORAGE_KEY_USER);
    setUser(null);
  }, []);

  return {
    user,
    isAuthenticated: user !== null,
    isLoading,
    error,
    login,
    logout,
  };
}
