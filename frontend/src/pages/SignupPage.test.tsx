import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { SignupPage } from "@/pages/SignupPage";
import { ToastProvider } from "@/components/toast";
import { clearTokens } from "@/features/auth/token";

const mockRegister = vi.fn();

vi.mock("@/features/auth", () => ({
  useAuth: () => ({
    user: null,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: mockRegister,
  }),
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  clearTokens();
  localStorage.clear();
  vi.clearAllMocks();
  // Default: registration mode is open (simplest case for most tests)
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ mode: "open" }),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderSignupPage(initialEntry = "/signup") {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <SignupPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("SignupPage", () => {
  test("renders signup form with all fields", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign Up" })).toBeInTheDocument();
  });

  test("renders link to login page", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByText("Login")).toBeInTheDocument();
    });
  });

  test("shows invite code field when invite_only", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ mode: "invite_only" }),
    });
    renderSignupPage();
    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("Enter invite code..."),
      ).toBeInTheDocument();
    });
  });

  test("hides invite code field when open registration", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Sign Up" }),
      ).toBeInTheDocument();
    });
    expect(screen.queryByPlaceholderText("Enter invite code...")).toBeNull();
  });

  test("fetches registration mode on mount", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/auth/registration-mode",
      );
    });
  });

  test("shows invite-only notice when registration is invite_only", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ mode: "invite_only" }),
    });
    renderSignupPage();
    await waitFor(() => {
      expect(
        screen.getByText("Registration is by invitation only."),
      ).toBeInTheDocument();
    });
  });

  test("does not show invite-only notice when registration is open", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });
    expect(
      screen.queryByText("Registration is by invitation only."),
    ).not.toBeInTheDocument();
  });

  test("shows validation error for short username", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "ab" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "password123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign Up" }));

    expect(
      screen.getByText("Username must be 3-20 characters"),
    ).toBeInTheDocument();
    expect(mockRegister).not.toHaveBeenCalled();
  });

  test("shows validation error for short password", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "short" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "short" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign Up" }));

    expect(
      screen.getByText("Password must be at least 8 characters"),
    ).toBeInTheDocument();
    expect(mockRegister).not.toHaveBeenCalled();
  });

  test("shows validation error when passwords do not match", async () => {
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "different1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign Up" }));

    expect(screen.getByText("Passwords do not match")).toBeInTheDocument();
    expect(mockRegister).not.toHaveBeenCalled();
  });

  test("calls register on valid form submit", async () => {
    mockRegister.mockResolvedValueOnce({ emailConfirmationRequired: false });
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "password123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign Up" }));

    await waitFor(() => {
      expect(mockRegister).toHaveBeenCalledWith({
        username: "testuser",
        email: "test@example.com",
        password: "password123",
        invite_code: undefined,
      });
    });
  });

  test("shows error toast on failed registration", async () => {
    mockRegister.mockRejectedValueOnce(new Error("Username already taken"));
    renderSignupPage();
    await waitFor(() => {
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "testuser" },
    });
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "password123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign Up" }));

    await waitFor(() => {
      expect(screen.getByText("Username already taken")).toBeInTheDocument();
    });
  });
});
