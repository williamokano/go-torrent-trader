import { useCallback, useEffect, useMemo, useState } from "react";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  setAccessToken,
  setRefreshToken,
} from "./token";
import { AuthContext } from "./AuthContextDef";
import type { User, RegisterData, AuthContextValue } from "./AuthContextDef";
import { api } from "@/api";

export type { User, RegisterData, AuthContextValue };

const ADMIN_GROUP_ID = 1;

function mapUser(
  profile: Record<string, unknown> | undefined,
): User | undefined {
  if (!profile) return undefined;
  const groupId = profile.group_id as number;
  return {
    id: profile.id as number,
    username: profile.username as string,
    email: profile.email as string,
    group_id: groupId,
    uploaded: profile.uploaded as number,
    downloaded: profile.downloaded as number,
    enabled: profile.enabled as boolean,
    created_at: profile.created_at as string,
    isAdmin: groupId === ADMIN_GROUP_ID,
  };
}

function getErrorMessage(error: unknown): string {
  if (
    error &&
    typeof error === "object" &&
    "error" in error &&
    typeof (error as Record<string, unknown>).error === "object"
  ) {
    const inner = (error as { error: { message?: string } }).error;
    if (inner?.message) return inner.message;
  }
  return "An unexpected error occurred";
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(() => !!getRefreshToken());

  const isAuthenticated = user !== null;

  useEffect(() => {
    let cancelled = false;

    async function tryRefresh() {
      const refreshToken = getRefreshToken();
      if (!refreshToken) {
        setIsLoading(false);
        return;
      }

      try {
        const { data, error } = await api.POST("/api/v1/auth/refresh", {
          body: { refresh_token: refreshToken },
        });

        if (error || !data?.tokens) {
          clearTokens();
          return;
        }

        if (!cancelled) {
          setAccessToken(data.tokens.access_token ?? null);
          setRefreshToken(data.tokens.refresh_token ?? null);

          const meRes = await api.GET("/api/v1/auth/me", {
            headers: {
              Authorization: `Bearer ${data.tokens.access_token}`,
            },
          });

          if (!cancelled && meRes.data?.user) {
            setUser(
              mapUser(meRes.data.user as Record<string, unknown>) ?? null,
            );
          }
        }
      } catch {
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
    const { data, error } = await api.POST("/api/v1/auth/login", {
      body: { username, password },
    });

    if (error) {
      throw new Error(getErrorMessage(error));
    }

    if (!data?.tokens || !data?.user) {
      throw new Error("Invalid response from server");
    }

    setAccessToken(data.tokens.access_token ?? null);
    setRefreshToken(data.tokens.refresh_token ?? null);
    setUser(mapUser(data.user as Record<string, unknown>) ?? null);
  }, []);

  const logout = useCallback(async () => {
    try {
      const token = getAccessToken();
      if (token) {
        await api.POST("/api/v1/auth/logout", {
          headers: { Authorization: `Bearer ${token}` },
        });
      }
    } finally {
      clearTokens();
      setUser(null);
    }
  }, []);

  const register = useCallback(async (data: RegisterData) => {
    const { data: resData, error } = await api.POST("/api/v1/auth/register", {
      body: {
        username: data.username,
        email: data.email,
        password: data.password,
      },
    });

    if (error) {
      throw new Error(getErrorMessage(error));
    }

    if (!resData?.tokens || !resData?.user) {
      throw new Error("Invalid response from server");
    }

    setAccessToken(resData.tokens.access_token ?? null);
    setRefreshToken(resData.tokens.refresh_token ?? null);
    setUser(mapUser(resData.user as Record<string, unknown>) ?? null);
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
