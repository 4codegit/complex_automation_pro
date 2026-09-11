import React, { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import type { WhoAmI } from '../api/types';

type AuthState =
  | { kind: 'loading' }
  | { kind: 'anonymous'; note: string }
  | { kind: 'logged_in'; user: WhoAmI };

interface AuthContextValue {
  state: AuthState;
  user: WhoAmI | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

// probeWhoami distinguishes "not logged in" (401) from "auth disabled"
// (404/501: an older server build without the login endpoint).
async function probeWhoami(): Promise<AuthState> {
  try {
    const user = await api.get<WhoAmI>('/access/whoami');
    return { kind: 'logged_in', user };
  } catch (err) {
    const status = (err as { status?: number }).status ?? 0;
    if (status === 401) {
      return { kind: 'anonymous', note: '' };
    }
    return { kind: 'anonymous', note: 'Аутентификация отключена (режим разработки)' };
  }
}

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [state, setState] = useState<AuthState>({ kind: 'loading' });

  const refresh = async () => setState(await probeWhoami());

  useEffect(() => {
    void refresh();
    const onUnauthorized = () => {
      setState((prev) => {
        if (prev.kind === 'logged_in') return { kind: 'anonymous', note: '' };
        return prev;
      });
    };
    window.addEventListener('cap:unauthorized', onUnauthorized);
    return () => window.removeEventListener('cap:unauthorized', onUnauthorized);
  }, []);

  const login = async (username: string, password: string) => {
    await api.post('/access/login', { username, password });
    const user = await api.get<WhoAmI>('/access/whoami');
    setState({ kind: 'logged_in', user });
  };

  const logout = async () => {
    try {
      await api.post('/access/logout');
    } catch {
      // best effort: clear the local view regardless
    }
    setState({ kind: 'anonymous', note: '' });
  };

  const value = useMemo<AuthContextValue>(
    () => ({
      state,
      user: state.kind === 'logged_in' ? state.user : null,
      login,
      logout,
      refresh,
    }),
    [state],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

export function useWhoAmI(): WhoAmI | null {
  return useAuth().user;
}
