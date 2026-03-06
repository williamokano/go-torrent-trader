import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { LoginPage } from "@/pages/LoginPage";
import { ToastProvider } from "@/components/toast";
import { clearTokens } from "@/features/auth/token";

const mockLogin = vi.fn();

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    login: mockLogin,
    logout: vi.fn(),
    register: vi.fn(),
  }),
}));

afterEach(cleanup);

beforeEach(() => {
  clearTokens();
  localStorage.clear();
  vi.clearAllMocks();
});

function renderLoginPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/login"]}>
        <LoginPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("LoginPage", () => {
  test("renders login form with username and password fields", () => {
    renderLoginPage();
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Login" })).toBeInTheDocument();
  });

  test("renders link to signup page", () => {
    renderLoginPage();
    expect(screen.getByText("Sign up")).toBeInTheDocument();
  });

  test("calls login on form submit", async () => {
    mockLogin.mockResolvedValueOnce(undefined);
    renderLoginPage();

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "testpass123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Login" }));

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith("testuser", "testpass123");
    });
  });

  test("shows error toast on failed login", async () => {
    mockLogin.mockRejectedValueOnce(new Error("Invalid credentials"));
    renderLoginPage();

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "wrongpass" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Login" }));

    await waitFor(() => {
      expect(screen.getByText("Invalid credentials")).toBeInTheDocument();
    });
  });

  test("shows loading state while submitting", async () => {
    let resolveLogin: () => void;
    mockLogin.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveLogin = resolve;
      }),
    );
    renderLoginPage();

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "testpass123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Login" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Logging in..." }),
      ).toBeDisabled();
    });

    resolveLogin!();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Login" })).not.toBeDisabled();
    });
  });
});
