import { useCallback, useEffect, useMemo, useState } from "react";
import {
  clearTokens,
  getRefreshToken,
  setAccessToken,
  setRefreshToken,
} from "./token";
import { AuthContext } from "./AuthContextDef";
import type { User, RegisterData, AuthContextValue } from "./AuthContextDef";

export type { User, RegisterData, AuthContextValue };

// Placeholder API calls - will be replaced with real openapi-fetch calls
// when BE-1.1/1.2 endpoints are built.

type AuthTokenResponse = {
  accessToken: string;
  refreshToken: string;
  user: User;
};

// Placeholder API calls - will be replaced with real openapi-fetch calls
// when BE-1.1/1.2 endpoints are built.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
async function apiLogin(_u: string, _p: string): Promise<AuthTokenResponse> {
  // TODO: Replace with actual API call: api.POST("/auth/login", { body: { username, password } })
  throw new Error("Auth API not implemented yet");
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
async function apiRegister(_data: RegisterData): Promise<AuthTokenResponse> {
  // TODO: Replace with actual API call: api.POST("/auth/register", { body: data })
  throw new Error("Auth API not implemented yet");
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
async function apiRefreshToken(_token: string): Promise<AuthTokenResponse> {
  // TODO: Replace with actual API call: api.POST("/auth/refresh", { body: { refreshToken } })
  throw new Error("Auth API not implemented yet");
}

async function apiLogout(): Promise<void> {
  // TODO: Replace with actual API call: api.POST("/auth/logout")
  // For now, just clear local state (no server call needed)
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  // Only start loading if there's a refresh token to try
  const [isLoading, setIsLoading] = useState(() => !!getRefreshToken());

  const isAuthenticated = user !== null;

  // Attempt silent refresh on mount
  useEffect(() => {
    let cancelled = false;

    async function tryRefresh() {
      const refreshToken = getRefreshToken();
      if (!refreshToken) {
        setIsLoading(false);
        return;
      }

      try {
        const result = await apiRefreshToken(refreshToken);
        if (!cancelled) {
          setAccessToken(result.accessToken);
          setRefreshToken(result.refreshToken);
          setUser(result.user);
        }
      } catch {
        // Refresh failed - clear stale tokens
        clearTokens();
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    tryRefresh();
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    const result = await apiLogin(username, password);
    setAccessToken(result.accessToken);
    setRefreshToken(result.refreshToken);
    setUser(result.user);
  }, []);

  const logout = useCallback(async () => {
    try {
      await apiLogout();
    } finally {
      clearTokens();
      setUser(null);
    }
  }, []);

  const register = useCallback(async (data: RegisterData) => {
    const result = await apiRegister(data);
    setAccessToken(result.accessToken);
    setRefreshToken(result.refreshToken);
    setUser(result.user);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      isAuthenticated,
      isLoading,
      login,
      logout,
      register,
    }),
    [user, isAuthenticated, isLoading, login, logout, register],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
