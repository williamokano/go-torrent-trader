import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { FormattingPage } from "@/pages/FormattingPage";

afterEach(cleanup);

function renderFormattingPage() {
  return render(
    <MemoryRouter>
      <FormattingPage />
    </MemoryRouter>,
  );
}

describe("FormattingPage", () => {
  test("renders page title", () => {
    renderFormattingPage();
    expect(screen.getByText("Formatting Reference")).toBeInTheDocument();
  });

  test("renders subtitle", () => {
    renderFormattingPage();
    expect(
      screen.getByText(/Use Markdown to format text/),
    ).toBeInTheDocument();
  });

  test("renders all section titles", () => {
    renderFormattingPage();
    expect(screen.getByText("Text Styling")).toBeInTheDocument();
    expect(screen.getByText("Headings")).toBeInTheDocument();
    expect(screen.getByText("Links & Images")).toBeInTheDocument();
    expect(screen.getByText("Code & Quotes")).toBeInTheDocument();
    expect(screen.getByText("Lists")).toBeInTheDocument();
    expect(screen.getByText("Tables")).toBeInTheDocument();
    expect(screen.getByText("Other")).toBeInTheDocument();
  });

  test("renders table headers", () => {
    renderFormattingPage();
    const formatHeaders = screen.getAllByText("Format");
    const syntaxHeaders = screen.getAllByText("Syntax");
    const previewHeaders = screen.getAllByText("Preview");
    expect(formatHeaders.length).toBeGreaterThan(0);
    expect(syntaxHeaders.length).toBeGreaterThan(0);
    expect(previewHeaders.length).toBeGreaterThan(0);
  });

  test("renders text styling examples", () => {
    renderFormattingPage();
    expect(screen.getByText("Bold")).toBeInTheDocument();
    expect(screen.getByText("Italic")).toBeInTheDocument();
    expect(screen.getByText("Strikethrough")).toBeInTheDocument();
    expect(screen.getByText("Bold & Italic")).toBeInTheDocument();
  });

  test("renders Markdown syntax strings", () => {
    renderFormattingPage();
    expect(screen.getByText("**bold text**")).toBeInTheDocument();
    expect(screen.getByText("*italic text*")).toBeInTheDocument();
    expect(screen.getByText("~~struck text~~")).toBeInTheDocument();
  });

  test("renders preview content", () => {
    renderFormattingPage();
    expect(screen.getByText("bold text")).toBeInTheDocument();
    expect(screen.getByText("italic text")).toBeInTheDocument();
    expect(screen.getByText("struck text")).toBeInTheDocument();
  });

  test("renders link examples", () => {
    renderFormattingPage();
    expect(screen.getByText("Link")).toBeInTheDocument();
    expect(screen.getByText("Auto-link")).toBeInTheDocument();
    expect(screen.getByText("Click here")).toBeInTheDocument();
  });

  test("renders code and quote examples", () => {
    renderFormattingPage();
    expect(screen.getByText("Inline code")).toBeInTheDocument();
    expect(screen.getByText("Code block")).toBeInTheDocument();
    expect(screen.getByText("Quote")).toBeInTheDocument();
  });

  test("renders list examples", () => {
    renderFormattingPage();
    expect(screen.getByText("Unordered list")).toBeInTheDocument();
    expect(screen.getByText("Ordered list")).toBeInTheDocument();
    expect(screen.getByText("Task list")).toBeInTheDocument();
  });

  test("renders note about combining syntax", () => {
    renderFormattingPage();
    expect(screen.getByText(/Syntax can be combined/)).toBeInTheDocument();
  });
});
