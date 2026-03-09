import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, test, expect } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { FAQPage } from "@/pages/FAQPage";

afterEach(cleanup);

function renderFAQPage() {
  return render(
    <MemoryRouter>
      <FAQPage />
    </MemoryRouter>,
  );
}

describe("FAQPage", () => {
  test("renders page title", () => {
    renderFAQPage();
    expect(screen.getByText("Frequently Asked Questions")).toBeInTheDocument();
  });

  test("renders subtitle", () => {
    renderFAQPage();
    expect(
      screen.getByText(
        "Find answers to common questions about using the tracker.",
      ),
    ).toBeInTheDocument();
  });

  test("renders all section titles", () => {
    renderFAQPage();
    expect(screen.getByText("Account")).toBeInTheDocument();
    expect(screen.getByText("Uploading")).toBeInTheDocument();
    expect(screen.getByText("Downloading")).toBeInTheDocument();
    expect(screen.getByText("Ratio")).toBeInTheDocument();
    expect(screen.getByText("Rules & Moderation")).toBeInTheDocument();
    expect(screen.getByText("Technical")).toBeInTheDocument();
  });

  test("renders questions as buttons", () => {
    renderFAQPage();
    const questionButton = screen.getByText("How do I create an account?");
    expect(questionButton.tagName).toBe("BUTTON");
  });

  test("answers are hidden by default", () => {
    renderFAQPage();
    expect(
      screen.queryByText(/Registration may be open or invite-only/),
    ).not.toBeInTheDocument();
  });

  test("clicking a question reveals the answer", () => {
    renderFAQPage();
    const question = screen.getByText("How do I create an account?");
    fireEvent.click(question);
    expect(
      screen.getByText(/Registration may be open or invite-only/),
    ).toBeInTheDocument();
  });

  test("clicking an open question hides the answer", () => {
    renderFAQPage();
    const question = screen.getByText("How do I create an account?");

    fireEvent.click(question);
    expect(
      screen.getByText(/Registration may be open or invite-only/),
    ).toBeInTheDocument();

    fireEvent.click(question);
    expect(
      screen.queryByText(/Registration may be open or invite-only/),
    ).not.toBeInTheDocument();
  });

  test("question buttons have aria-expanded attribute", () => {
    renderFAQPage();
    const question = screen.getByText("How do I create an account?");
    expect(question).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(question);
    expect(question).toHaveAttribute("aria-expanded", "true");
  });

  test("multiple questions can be open simultaneously", () => {
    renderFAQPage();
    const q1 = screen.getByText("How do I create an account?");
    const q2 = screen.getByText("What is a passkey?");

    fireEvent.click(q1);
    fireEvent.click(q2);

    expect(
      screen.getByText(/Registration may be open or invite-only/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/unique identifier included in your torrent/),
    ).toBeInTheDocument();
  });
});
