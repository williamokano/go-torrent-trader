import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, test } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { CheckEmailPage } from "@/pages/CheckEmailPage";

afterEach(cleanup);

describe("CheckEmailPage", () => {
  test("renders check email message", () => {
    render(
      <MemoryRouter>
        <CheckEmailPage />
      </MemoryRouter>,
    );
    expect(screen.getByText("Check Your Email")).toBeInTheDocument();
    expect(
      screen.getByText("The link will expire in 24 hours."),
    ).toBeInTheDocument();
  });

  test("renders resend and login links", () => {
    render(
      <MemoryRouter>
        <CheckEmailPage />
      </MemoryRouter>,
    );
    expect(screen.getByText("Resend confirmation email")).toBeInTheDocument();
    expect(screen.getByText("Login")).toBeInTheDocument();
  });

  test("shows generic message when no email in state", () => {
    render(
      <MemoryRouter>
        <CheckEmailPage />
      </MemoryRouter>,
    );
    expect(
      screen.getByText(/We've sent a confirmation email to your address/),
    ).toBeInTheDocument();
  });
});
