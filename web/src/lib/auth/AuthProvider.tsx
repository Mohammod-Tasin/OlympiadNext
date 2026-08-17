"use client";

import { createContext, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import * as authApi from "@/lib/api/authApi";
import { ApiError } from "@/lib/api/client";
import type { AuthStatus, User } from "@/types/auth";

interface AuthContextValue {
  status: AuthStatus;
  user: User | null;
  accessToken: string | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  loginWithGoogle: (idToken: string) => Promise<void>;
  logout: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

// Refresh a little before actual expiry so an in-flight request never
// races an access token that just died.
const REFRESH_SKEW_MS = 30_000;

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<User | null>(null);
  const [accessToken, setAccessToken] = useState<string | null>(null);
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearScheduledRefresh = () => {
    if (refreshTimer.current) {
      clearTimeout(refreshTimer.current);
      refreshTimer.current = null;
    }
  };

  const applySession = useCallback(async (token: string, expiresAt: string) => {
    setAccessToken(token);
    const user = await authApi.me(token);
    setUser(user);
    setStatus("authenticated");

    clearScheduledRefresh();
    const delay = Math.max(new Date(expiresAt).getTime() - Date.now() - REFRESH_SKEW_MS, 0);
    refreshTimer.current = setTimeout(() => {
      void silentRefresh();
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, delay);
  }, []);

  const silentRefresh = useCallback(async () => {
    try {
      const res = await authApi.refresh();
      await applySession(res.access_token, res.access_token_expires_at);
    } catch {
      clearScheduledRefresh();
      setAccessToken(null);
      setUser(null);
      setStatus("unauthenticated");
    }
  }, [applySession]);

  useEffect(() => {
    void silentRefresh();
    return clearScheduledRefresh;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      const res = await authApi.login(email, password);
      await applySession(res.access_token, res.access_token_expires_at);
    },
    [applySession],
  );

  const register = useCallback(
    async (email: string, password: string) => {
      const res = await authApi.register(email, password);
      await applySession(res.access_token, res.access_token_expires_at);
    },
    [applySession],
  );

  const loginWithGoogle = useCallback(
    async (idToken: string) => {
      const res = await authApi.loginWithGoogle(idToken);
      await applySession(res.access_token, res.access_token_expires_at);
    },
    [applySession],
  );

  const logout = useCallback(async () => {
    clearScheduledRefresh();
    try {
      await authApi.logout();
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
    }
    setAccessToken(null);
    setUser(null);
    setStatus("unauthenticated");
  }, []);

  return (
    <AuthContext.Provider value={{ status, user, accessToken, login, register, loginWithGoogle, logout }}>
      {children}
    </AuthContext.Provider>
  );
}
