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
  RegisterResult,
  AuthContextValue,
} from "./AuthContextDef";
import { api } from "@/api";

export type {
  User,
  UserPermissions,
  RegisterData,
  RegisterResult,
  AuthContextValue,
};

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
    can_download: (profile.can_download as boolean) ?? true,
    can_upload: (profile.can_upload as boolean) ?? true,
    can_chat: (profile.can_chat as boolean) ?? true,
    created_at: (profile.created_at as string) ?? "",
    last_login: (profile.last_login as string) ?? "",
    isAdmin,
    isStaff: isAdmin || isModerator,
    permissions: perms,
  };
}

import { ApiError } from "./ApiError";

function throwApiError(error: unknown): never {
  if (
    error &&
    typeof error === "object" &&
    "error" in error &&
    typeof (error as Record<string, unknown>).error === "object"
  ) {
    const inner = (error as { error: { code?: string; message?: string } })
      .error;
    if (inner?.code || inner?.message) {
      throw new ApiError(
        inner.code ?? "unknown",
        inner.message ?? "An unexpected error occurred",
      );
    }
  }
  throw new ApiError("unknown", "An unexpected error occurred");
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

/**
 * Outcome of a bootstrap `/auth/me` fetch, split so callers can distinguish a
 * genuine auth failure from a transient hiccup:
 *   - "user"        — a profile came back; the session is good.
 *   - "unauthorized"— HTTP 401; the token is genuinely rejected.
 *   - "transient"   — network/transport error, a non-401 status (5xx), or a
 *                     2xx with an empty/unparseable body. The token might still
 *                     be fine, so the session must be kept and retried later.
 */
type MeResult =
  | { kind: "user"; user: User | null }
  | { kind: "unauthorized" }
  | { kind: "transient" };

async function fetchMeProfile(accessToken: string): Promise<MeResult> {
  try {
    const meRes = await api.GET("/api/v1/auth/me", {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    if (meRes.data?.user) {
      return {
        kind: "user",
        user: mapUser(meRes.data.user as Record<string, unknown>) ?? null,
      };
    }
    if (meRes.response?.status === 401) {
      return { kind: "unauthorized" };
    }
    // Non-401 status or an empty/200 body: not an auth failure, keep the token.
    return { kind: "transient" };
  } catch {
    // Thrown = transport/network error: keep the token.
    return { kind: "transient" };
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
      // Case 1: Access token is still valid — fetch the profile. Only a real
      // 401 falls through to the refresh path; any other failure keeps the
      // session so a later reload can recover.
      if (isAccessTokenValid()) {
        const result = await fetchMeProfile(getAccessToken() ?? "");
        if (result.kind === "user") {
          setUser(result.user);
          setIsLoading(false);
          return;
        }
        if (result.kind === "transient") {
          // Non-auth hiccup (network error, 5xx, empty body). Keep the tokens.
          setIsLoading(false);
          return;
        }
        // result.kind === "unauthorized": token genuinely bad — try to refresh.
      }

      // Case 2: Refresh the session. This is reached either because the access
      // token is expired, or because Case 1 got a genuine 401.
      const refreshToken = getRefreshToken();
      if (!refreshToken) {
        clearTokens();
        setIsLoading(false);
        return;
      }

      try {
        const { data, error, response } = await api.POST(
          "/api/v1/auth/refresh",
          { body: { refresh_token: refreshToken } },
        );

        if (response?.status === 401 || response?.status === 403) {
          // Refresh token genuinely invalid/expired — a real logout.
          clearTokens();
          setIsLoading(false);
          return;
        }

        if (error || !data?.tokens) {
          // Non-auth failure (network blip, 5xx). Keep the tokens.
          setIsLoading(false);
          return;
        }

        storeTokens(data.tokens);

        const result = await fetchMeProfile(data.tokens.access_token ?? "");
        if (result.kind === "user") {
          setUser(result.user);
        } else if (result.kind === "unauthorized") {
          // Freshly refreshed token still rejected — a real auth failure.
          clearTokens();
        }
        // "transient": keep the tokens; user stays null and recovers on reload.
        setIsLoading(false);
      } catch {
        // Transport error on refresh — keep the tokens.
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
      throwApiError(error);
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

  const register = useCallback(
    async (data: RegisterData): Promise<RegisterResult> => {
      const { data: resData, error } = await api.POST("/api/v1/auth/register", {
        body: {
          username: data.username,
          email: data.email,
          password: data.password,
          invite_code: data.invite_code,
        },
      });

      if (error) {
        throwApiError(error);
      }

      // Check if email confirmation is required
      if (resData?.email_confirmation_required) {
        return { emailConfirmationRequired: true };
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

      return { emailConfirmationRequired: false };
    },
    [],
  );

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
