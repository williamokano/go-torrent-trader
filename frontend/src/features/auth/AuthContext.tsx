import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  isAccessTokenValid,
  setAccessToken,
  setRefreshToken,
  setTokenExpiry,
} from "./token";
import { AuthContext } from "./AuthContextDef";
import type {
  User,
  UserPermissions,
  RegisterData,
  AuthContextValue,
} from "./AuthContextDef";
import { api } from "@/api";

export type { User, UserPermissions, RegisterData, AuthContextValue };

function mapUser(
  profile: Record<string, unknown> | undefined,
): User | undefined {
  if (!profile) return undefined;
  const perms = profile.permissions as UserPermissions | undefined;
  const isAdmin = perms?.is_admin ?? false;
  const isModerator = perms?.is_moderator ?? false;
  return {
    id: profile.id as number,
    username: profile.username as string,
    email: (profile.email as string) ?? "",
    group_id: profile.group_id as number,
    avatar: (profile.avatar as string) ?? "",
    title: (profile.title as string) ?? "",
    info: (profile.info as string) ?? "",
    uploaded: (profile.uploaded as number) ?? 0,
    downloaded: (profile.downloaded as number) ?? 0,
    ratio: (profile.ratio as number) ?? 0,
    passkey: (profile.passkey as string) ?? "",
    invites: (profile.invites as number) ?? 0,
    warned: (profile.warned as boolean) ?? false,
    donor: (profile.donor as boolean) ?? false,
    enabled: (profile.enabled as boolean) ?? true,
    created_at: (profile.created_at as string) ?? "",
    last_login: (profile.last_login as string) ?? "",
    isAdmin,
    isStaff: isAdmin || isModerator,
    permissions: perms,
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

function storeTokens(tokens: {
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
}) {
  setAccessToken(tokens.access_token ?? null);
  setRefreshToken(tokens.refresh_token ?? null);
  if (tokens.expires_in) {
    setTokenExpiry(tokens.expires_in);
  }
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(
    () => !!getAccessToken() || !!getRefreshToken(),
  );
  const restoringRef = useRef(false);

  const isAuthenticated = user !== null;

  // Restore session on mount
  useEffect(() => {
    // Prevent double-run in React StrictMode
    if (restoringRef.current) return;
    restoringRef.current = true;

    async function restoreSession() {
      // Case 1: Access token is still valid — just fetch user profile
      if (isAccessTokenValid()) {
        try {
          const meRes = await api.GET("/api/v1/auth/me", {
            headers: { Authorization: `Bearer ${getAccessToken()}` },
          });
          if (meRes.data?.user) {
            setUser(
              mapUser(meRes.data.user as Record<string, unknown>) ?? null,
            );
          } else {
            clearTokens();
          }
        } catch {
          clearTokens();
        } finally {
          setIsLoading(false);
        }
        return;
      }

      // Case 2: Access token expired but refresh token exists — refresh
      const refreshToken = getRefreshToken();
      if (!refreshToken) {
        clearTokens();
        setIsLoading(false);
        return;
      }

      try {
        const { data, error } = await api.POST("/api/v1/auth/refresh", {
          body: { refresh_token: refreshToken },
        });

        if (error || !data?.tokens) {
          clearTokens();
          setIsLoading(false);
          return;
        }

        storeTokens(data.tokens);

        const meRes = await api.GET("/api/v1/auth/me", {
          headers: { Authorization: `Bearer ${data.tokens.access_token}` },
        });

        if (meRes.data?.user) {
          setUser(mapUser(meRes.data.user as Record<string, unknown>) ?? null);
        } else {
          clearTokens();
        }
      } catch {
        clearTokens();
      } finally {
        setIsLoading(false);
      }
    }

    restoreSession();
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

    storeTokens(data.tokens);

    // Fetch full profile with permissions so isAdmin/isStaff are set immediately
    const meRes = await api.GET("/api/v1/auth/me", {
      headers: { Authorization: `Bearer ${data.tokens.access_token}` },
    });
    if (meRes.data?.user) {
      setUser(mapUser(meRes.data.user as Record<string, unknown>) ?? null);
    } else {
      setUser(mapUser(data.user as Record<string, unknown>) ?? null);
    }
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

  const refreshUser = useCallback(async () => {
    const token = getAccessToken();
    if (!token) return;
    const meRes = await api.GET("/api/v1/auth/me", {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (meRes.data?.user) {
      setUser(mapUser(meRes.data.user as Record<string, unknown>) ?? null);
    }
  }, []);

  const register = useCallback(async (data: RegisterData) => {
    const { data: resData, error } = await api.POST("/api/v1/auth/register", {
      body: {
        username: data.username,
        email: data.email,
        password: data.password,
        invite_code: data.invite_code,
      },
    });

    if (error) {
      throw new Error(getErrorMessage(error));
    }

    if (!resData?.tokens || !resData?.user) {
      throw new Error("Invalid response from server");
    }

    storeTokens(resData.tokens);

    const meRes = await api.GET("/api/v1/auth/me", {
      headers: { Authorization: `Bearer ${resData.tokens.access_token}` },
    });
    if (meRes.data?.user) {
      setUser(mapUser(meRes.data.user as Record<string, unknown>) ?? null);
    } else {
      setUser(mapUser(resData.user as Record<string, unknown>) ?? null);
    }
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      isAuthenticated,
      isLoading,
      login,
      logout,
      register,
      refreshUser,
    }),
    [user, isAuthenticated, isLoading, login, logout, register, refreshUser],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
