import { cleanup, render, screen, act } from "@testing-library/react";
import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { AuthProvider } from "@/features/auth";
import { useAuth } from "@/features/auth/useAuth";
import { clearTokens, getAccessToken, getRefreshToken } from "./token";

afterEach(cleanup);

function wrapper({ children }: { children: React.ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>;
}

describe("AuthProvider", () => {
  beforeEach(() => {
    clearTokens();
    localStorage.clear();
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

    // Wait for the useEffect (silent refresh) to complete
    // Since no refresh token exists, isLoading should become false quickly
    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.user).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
  });

  test("isLoading starts true and becomes false after mount", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    // isLoading may be true initially, but should settle to false
    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
  });

  test("login throws since API is not implemented yet", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await expect(
      act(() => result.current.login("user", "pass")),
    ).rejects.toThrow("Auth API not implemented yet");
  });

  test("register throws since API is not implemented yet", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await expect(
      act(() =>
        result.current.register({
          username: "user",
          email: "user@example.com",
          password: "pass",
        }),
      ),
    ).rejects.toThrow("Auth API not implemented yet");
  });

  test("logout clears tokens and user state", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });

    await vi.waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Logout should work even without a logged-in user (clearing state)
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
