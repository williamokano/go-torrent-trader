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
      screen.getByText(/Use BBCode tags to format text/),
    ).toBeInTheDocument();
  });

  test("renders all section titles", () => {
    renderFormattingPage();
    expect(screen.getByText("Text Styling")).toBeInTheDocument();
    expect(screen.getByText("Links & Images")).toBeInTheDocument();
    expect(screen.getByText("Code & Quotes")).toBeInTheDocument();
    expect(screen.getByText("Colors & Sizes")).toBeInTheDocument();
    expect(screen.getByText("Lists")).toBeInTheDocument();
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
    expect(screen.getByText("Underline")).toBeInTheDocument();
    expect(screen.getByText("Strikethrough")).toBeInTheDocument();
  });

  test("renders syntax strings", () => {
    renderFormattingPage();
    expect(screen.getByText("[b]bold text[/b]")).toBeInTheDocument();
    expect(screen.getByText("[i]italic text[/i]")).toBeInTheDocument();
    expect(screen.getByText("[u]underlined text[/u]")).toBeInTheDocument();
  });

  test("renders preview content", () => {
    renderFormattingPage();
    expect(screen.getByText("bold text")).toBeInTheDocument();
    expect(screen.getByText("italic text")).toBeInTheDocument();
    expect(screen.getByText("underlined text")).toBeInTheDocument();
  });

  test("renders link examples", () => {
    renderFormattingPage();
    expect(screen.getByText("URL (auto label)")).toBeInTheDocument();
    expect(screen.getByText("URL (custom label)")).toBeInTheDocument();
    expect(screen.getByText("Click here")).toBeInTheDocument();
  });

  test("renders code and quote examples", () => {
    renderFormattingPage();
    expect(screen.getByText("Inline code")).toBeInTheDocument();
    expect(screen.getByText("Code block")).toBeInTheDocument();
    expect(screen.getByText("Quote")).toBeInTheDocument();
  });

  test("renders color examples", () => {
    renderFormattingPage();
    expect(screen.getByText("red text")).toBeInTheDocument();
    expect(screen.getByText("green text")).toBeInTheDocument();
  });

  test("renders list examples", () => {
    renderFormattingPage();
    expect(screen.getByText("Unordered list")).toBeInTheDocument();
    expect(screen.getByText("Ordered list")).toBeInTheDocument();
  });

  test("renders note about nesting", () => {
    renderFormattingPage();
    expect(screen.getByText(/Tags can be nested/)).toBeInTheDocument();
  });
});
