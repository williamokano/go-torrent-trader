import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ConfirmEmailPage } from "@/pages/ConfirmEmailPage";

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage(search = "") {
  return render(
    <MemoryRouter initialEntries={[`/confirm-email${search}`]}>
      <ConfirmEmailPage />
    </MemoryRouter>,
  );
}

describe("ConfirmEmailPage", () => {
  test("shows error when no token provided", () => {
    renderPage();
    expect(
      screen.getByText("No confirmation token provided."),
    ).toBeInTheDocument();
    expect(screen.getByText("Resend confirmation email")).toBeInTheDocument();
  });

  test("shows confirm button, then success on click", async () => {
    const user = userEvent.setup();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ message: "confirmed" }),
    });

    renderPage("?token=validtoken123");

    // Should show the confirm button, not auto-fire
    const button = screen.getByRole("button", { name: "Confirm Email" });
    expect(button).toBeInTheDocument();

    await user.click(button);

    await waitFor(() => {
      expect(
        screen.getByText("Your email has been confirmed. You can now log in."),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("Go to Login")).toBeInTheDocument();

    // Verify it sent a POST with JSON body
    expect(mockFetch).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/auth/confirm-email",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: "validtoken123" }),
      }),
    );
  });

  test("shows error on invalid token", async () => {
    const user = userEvent.setup();
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: () =>
        Promise.resolve({
          error: { message: "invalid or expired confirmation link" },
        }),
    });

    renderPage("?token=badtoken");

    await user.click(screen.getByRole("button", { name: "Confirm Email" }));

    await waitFor(() => {
      expect(
        screen.getByText("invalid or expired confirmation link"),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("Resend confirmation email")).toBeInTheDocument();
  });

  test("shows fallback error on network failure", async () => {
    const user = userEvent.setup();
    mockFetch.mockRejectedValueOnce(new Error("network error"));

    renderPage("?token=sometoken");

    await user.click(screen.getByRole("button", { name: "Confirm Email" }));

    await waitFor(() => {
      expect(
        screen.getByText("Failed to confirm email. Please try again."),
      ).toBeInTheDocument();
    });
  });
});
