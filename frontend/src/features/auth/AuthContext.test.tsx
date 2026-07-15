import { cleanup, render, screen, act } from "@testing-library/react";
import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { AuthProvider } from "@/features/auth";
import { useAuth } from "@/features/auth/useAuth";
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  setAccessToken,
  setRefreshToken,
  setTokenExpiry,
} from "./token";

const mockPost = vi.fn();
const mockGet = vi.fn();

vi.mock("@/api", () => ({
  api: {
    POST: (...args: unknown[]) => mockPost(...args),
    GET: (...args: unknown[]) => mockGet(...args),
  },
}));

afterEach(cleanup);

function wrapper({ children }: { children: React.ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>;
}

describe("AuthProvider", () => {
  beforeEach(() => {
    clearTokens();
    localStorage.clear();
    vi.clearAllMocks();
    mockPost.mockResolvedValue({
      data: null,
      error: { error: { message: "not implemented" } },
    });
    mockGet.mockResolvedValue({
      data: null,
      error: { error: { message: "not implemented" } },
    });
  });

  test("renders children", () => {
    render(
      <AuthProvider>
        <div>Child content</div>
      </AuthProvider>,
    );
    expect(screen.getByText("Child content")).toBeInTheDocument();
  });

  test("initial state has no user and is not authenticated", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
  });

  test("isLoading starts true and becomes false after mount", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
  });

  test("login stores tokens and user on success", async () => {
    mockPost.mockResolvedValueOnce({
      data: {
        user: {
          id: 1,
          username: "testuser",
          email: "test@example.com",
          group_id: 1,
          uploaded: 0,
          downloaded: 0,
          enabled: true,
          can_download: true,
          can_upload: true,
          can_chat: true,
          created_at: "2026-01-01T00:00:00Z",
        },
        tokens: {
          access_token: "access123",
          refresh_token: "refresh123",
          expires_in: 3600,
        },
      },
      error: undefined,
    });

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await act(() => result.current.login("testuser", "pass"));

    expect(result.current.user?.username).toBe("testuser");
    expect(result.current.isAuthenticated).toBe(true);
    expect(getAccessToken()).toBe("access123");
    expect(getRefreshToken()).toBe("refresh123");
  });

  test("login throws on API error", async () => {
    mockPost.mockResolvedValueOnce({
      data: undefined,
      error: {
        error: { code: "INVALID_CREDENTIALS", message: "Invalid credentials" },
      },
    });

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await expect(
      act(() => result.current.login("user", "wrongpass")),
    ).rejects.toThrow("Invalid credentials");
  });

  test("logout clears tokens and user state", async () => {
    mockPost
      .mockResolvedValueOnce({
        data: {
          user: {
            id: 1,
            username: "testuser",
            email: "test@example.com",
            group_id: 1,
            uploaded: 0,
            downloaded: 0,
            enabled: true,
            can_download: true,
            can_upload: true,
            can_chat: true,
            created_at: "2026-01-01T00:00:00Z",
          },
          tokens: {
            access_token: "access123",
            refresh_token: "refresh123",
            expires_in: 3600,
          },
        },
        error: undefined,
      })
      .mockResolvedValueOnce({ data: undefined, error: undefined });

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await act(() => result.current.login("testuser", "pass"));
    expect(result.current.isAuthenticated).toBe(true);

    await act(() => result.current.logout());

    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
  });

  test("provides login, logout, and register functions", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.login).toBeTypeOf("function");
    expect(result.current.logout).toBeTypeOf("function");
    expect(result.current.register).toBeTypeOf("function");
    expect(result.current.refreshUser).toBeTypeOf("function");
  });
});

const sampleUser = {
  id: 1,
  username: "testuser",
  email: "test@example.com",
  group_id: 1,
  uploaded: 0,
  downloaded: 0,
  enabled: true,
  can_download: true,
  can_upload: true,
  can_chat: true,
  created_at: "2026-01-01T00:00:00Z",
};

/** Seed a valid, non-expired access token plus a refresh token. */
function seedValidSession() {
  setAccessToken("valid-access");
  setRefreshToken("valid-refresh");
  setTokenExpiry(3600); // well beyond the 5-minute refresh buffer
}

describe("AuthProvider session restore", () => {
  beforeEach(() => {
    clearTokens();
    localStorage.clear();
    vi.clearAllMocks();
    mockPost.mockResolvedValue({
      data: null,
      error: { error: { message: "not implemented" } },
    });
    mockGet.mockResolvedValue({
      data: null,
      error: { error: { message: "not implemented" } },
    });
  });

  test("a /auth/me that returns 200 with no user keeps the tokens and stays unauthenticated", async () => {
    seedValidSession();
    // Valid access token -> Case 1. Empty 200 body (the backend +Inf bug).
    mockGet.mockResolvedValueOnce({
      data: {},
      error: undefined,
      response: { status: 200 },
    });

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Not logged out: tokens survive, user is simply null (recovers on reload).
    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
    expect(getAccessToken()).toBe("valid-access");
    expect(getRefreshToken()).toBe("valid-refresh");
    // A non-401 must not attempt a refresh.
    expect(mockPost).not.toHaveBeenCalled();
  });

  test("a /auth/me 401 with a valid refresh token refreshes and restores the session", async () => {
    seedValidSession();
    // Case 1 -> 401 on /me falls through to refresh.
    mockGet
      .mockResolvedValueOnce({
        data: undefined,
        error: { error: { message: "unauthorized" } },
        response: { status: 401 },
      })
      // Case 2 -> /me with the freshly refreshed token returns the profile.
      .mockResolvedValueOnce({
        data: { user: sampleUser },
        error: undefined,
        response: { status: 200 },
      });
    mockPost.mockResolvedValueOnce({
      data: {
        tokens: {
          access_token: "new-access",
          refresh_token: "new-refresh",
          expires_in: 3600,
        },
      },
      error: undefined,
      response: { status: 200 },
    });

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.user?.username).toBe("testuser");
    expect(result.current.isAuthenticated).toBe(true);
    expect(mockPost).toHaveBeenCalledTimes(1);
    // New tokens from the refresh are persisted.
    expect(getAccessToken()).toBe("new-access");
    expect(getRefreshToken()).toBe("new-refresh");
  });

  test("a genuine 401 on /auth/refresh clears the tokens (real logout)", async () => {
    // No valid access token -> Case 2 (refresh) straight away.
    setRefreshToken("expired-refresh");
    mockPost.mockResolvedValueOnce({
      data: undefined,
      error: { error: { message: "invalid refresh token" } },
      response: { status: 401 },
    });

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
  });

  test("a transport error during restore keeps the tokens", async () => {
    seedValidSession();
    // Valid access token -> Case 1. A thrown error is a network hiccup.
    mockGet.mockRejectedValueOnce(new Error("network down"));

    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
    expect(getAccessToken()).toBe("valid-access");
    expect(getRefreshToken()).toBe("valid-refresh");
    expect(mockPost).not.toHaveBeenCalled();
  });
});

describe("useAuth", () => {
  test("throws when used outside AuthProvider", () => {
    expect(() => {
      renderHook(() => useAuth());
    }).toThrow("useAuth must be used within an AuthProvider");
  });
});
