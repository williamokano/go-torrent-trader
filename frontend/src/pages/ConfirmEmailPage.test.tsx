import { cleanup, render, screen, waitFor } from "@testing-library/react";
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

  test("shows loading then success on valid token", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ message: "confirmed" }),
    });

    renderPage("?token=validtoken123");

    expect(
      screen.getByText("Confirming your email address..."),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(
        screen.getByText("Your email has been confirmed. You can now log in."),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("Go to Login")).toBeInTheDocument();
  });

  test("shows error on invalid token", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: () =>
        Promise.resolve({
          error: { message: "invalid or expired confirmation link" },
        }),
    });

    renderPage("?token=badtoken");

    await waitFor(() => {
      expect(
        screen.getByText("invalid or expired confirmation link"),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("Resend confirmation email")).toBeInTheDocument();
  });

  test("shows fallback error on network failure", async () => {
    mockFetch.mockRejectedValueOnce(new Error("network error"));

    renderPage("?token=sometoken");

    await waitFor(() => {
      expect(
        screen.getByText("Failed to confirm email. Please try again."),
      ).toBeInTheDocument();
    });
  });
});
