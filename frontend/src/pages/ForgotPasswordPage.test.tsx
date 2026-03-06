import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ForgotPasswordPage } from "@/pages/ForgotPasswordPage";
import { ToastProvider } from "@/components/toast";

const mockPost = vi.fn();

vi.mock("@/api", () => ({
  api: {
    POST: (...args: unknown[]) => mockPost(...args),
  },
}));

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/forgot-password"]}>
        <ForgotPasswordPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("ForgotPasswordPage", () => {
  test("renders form with email field and submit button", () => {
    renderPage();
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Send Reset Link" }),
    ).toBeInTheDocument();
  });

  test("renders link back to login", () => {
    renderPage();
    expect(screen.getByText("Back to login")).toBeInTheDocument();
  });

  test("shows success message after submit regardless of API response", async () => {
    mockPost.mockResolvedValueOnce({ data: {} });
    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "user@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send Reset Link" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "If this email exists, a reset link has been sent. Check your inbox.",
        ),
      ).toBeInTheDocument();
    });
  });

  test("shows success message even when API call fails", async () => {
    mockPost.mockRejectedValueOnce(new Error("Network error"));
    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "nonexistent@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send Reset Link" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "If this email exists, a reset link has been sent. Check your inbox.",
        ),
      ).toBeInTheDocument();
    });
  });

  test("calls API with email on submit", async () => {
    mockPost.mockResolvedValueOnce({ data: {} });
    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "user@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send Reset Link" }));

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledWith("/api/v1/auth/forgot-password", {
        body: { email: "user@example.com" },
      });
    });
  });

  test("shows loading state while submitting", async () => {
    let resolvePost: (value: unknown) => void;
    mockPost.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePost = resolve;
      }),
    );
    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "user@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send Reset Link" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Sending..." })).toBeDisabled();
    });

    resolvePost!({ data: {} });

    await waitFor(() => {
      expect(
        screen.getByText(
          "If this email exists, a reset link has been sent. Check your inbox.",
        ),
      ).toBeInTheDocument();
    });
  });
});
