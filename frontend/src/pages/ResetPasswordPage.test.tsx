import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ResetPasswordPage } from "@/pages/ResetPasswordPage";
import { ToastProvider } from "@/components/toast";

const mockPost = vi.fn();
const mockNavigate = vi.fn();

vi.mock("@/api", () => ({
  api: {
    POST: (...args: unknown[]) => mockPost(...args),
  },
}));

vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

beforeEach(() => {
  vi.clearAllMocks();
});

function renderPage(token = "valid-token") {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={[`/reset-password?token=${token}`]}>
        <ResetPasswordPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("ResetPasswordPage", () => {
  test("renders form with password fields", () => {
    renderPage();
    expect(screen.getByLabelText("New Password")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Reset Password" }),
    ).toBeInTheDocument();
  });

  test("renders link back to login", () => {
    renderPage();
    expect(screen.getByText("Back to login")).toBeInTheDocument();
  });

  test("shows validation error for short password", async () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "short" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "short" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reset Password" }));

    expect(
      screen.getByText("Password must be at least 8 characters"),
    ).toBeInTheDocument();
    expect(mockPost).not.toHaveBeenCalled();
  });

  test("shows validation error when passwords do not match", async () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "password123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "different1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reset Password" }));

    expect(screen.getByText("Passwords do not match")).toBeInTheDocument();
    expect(mockPost).not.toHaveBeenCalled();
  });

  test("shows success message and redirects on successful reset", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    mockPost.mockResolvedValueOnce({ data: {}, error: undefined });
    renderPage();

    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reset Password" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Your password has been reset. Redirecting to login...",
        ),
      ).toBeInTheDocument();
    });

    vi.advanceTimersByTime(3000);

    expect(mockNavigate).toHaveBeenCalledWith("/login", { replace: true });
  });

  test("calls API with token and password on submit", async () => {
    mockPost.mockResolvedValueOnce({ data: {}, error: undefined });
    renderPage("my-reset-token");

    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reset Password" }));

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledWith("/api/v1/auth/reset-password", {
        body: { token: "my-reset-token", password: "newpassword123" },
      });
    });
  });

  test("shows error toast on invalid/expired token", async () => {
    mockPost.mockResolvedValueOnce({
      data: undefined,
      error: { error: { message: "Token is invalid or expired" } },
    });
    renderPage();

    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reset Password" }));

    await waitFor(() => {
      expect(
        screen.getByText("Token is invalid or expired"),
      ).toBeInTheDocument();
    });
  });

  test("shows generic error toast on network failure", async () => {
    mockPost.mockRejectedValueOnce(new Error("Network error"));
    renderPage();

    fireEvent.change(screen.getByLabelText("New Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm Password"), {
      target: { value: "newpassword123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reset Password" }));

    await waitFor(() => {
      expect(
        screen.getByText("Failed to reset password. Please try again."),
      ).toBeInTheDocument();
    });
  });
});
