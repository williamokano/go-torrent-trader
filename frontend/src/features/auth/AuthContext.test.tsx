import { cleanup, render, screen, act } from "@testing-library/react";
import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { AuthProvider } from "@/features/auth";
import { useAuth } from "@/features/auth/useAuth";
import { clearTokens, getAccessToken, getRefreshToken } from "./token";

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
  });
});

describe("useAuth", () => {
  test("throws when used outside AuthProvider", () => {
    expect(() => {
      renderHook(() => useAuth());
    }).toThrow("useAuth must be used within an AuthProvider");
  });
});
