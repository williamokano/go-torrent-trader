import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ResendConfirmationPage } from "@/pages/ResendConfirmationPage";

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage() {
  return render(
    <MemoryRouter>
      <ResendConfirmationPage />
    </MemoryRouter>,
  );
}

describe("ResendConfirmationPage", () => {
  test("renders the form", () => {
    renderPage();
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Resend Confirmation Email" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Login")).toBeInTheDocument();
  });

  test("shows success message on successful resend", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          message:
            "If this email has a pending confirmation, a new link has been sent",
        }),
    });

    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Resend Confirmation Email" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "If this email has a pending confirmation, a new link has been sent",
        ),
      ).toBeInTheDocument();
    });
  });

  test("shows rate limit error", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 429,
      json: () =>
        Promise.resolve({
          error: {
            message:
              "please wait 5 minutes before requesting another confirmation email",
          },
        }),
    });

    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "test@example.com" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Resend Confirmation Email" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          "please wait 5 minutes before requesting another confirmation email",
        ),
      ).toBeInTheDocument();
    });
  });

  test("shows already confirmed error", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: () =>
        Promise.resolve({
          error: { message: "this account is already confirmed" },
        }),
    });

    renderPage();

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "confirmed@example.com" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Resend Confirmation Email" }),
    );

    await waitFor(() => {
      expect(
        screen.getByText("this account is already confirmed"),
      ).toBeInTheDocument();
    });
  });

  test("renders description text", () => {
    renderPage();
    expect(
      screen.getByText(
        "Enter your email address and we will send a new confirmation link.",
      ),
    ).toBeInTheDocument();
  });
});
