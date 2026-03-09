import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, test, expect } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { RulesPage } from "@/pages/RulesPage";

afterEach(cleanup);

function renderRulesPage() {
  return render(
    <MemoryRouter>
      <RulesPage />
    </MemoryRouter>,
  );
}

describe("RulesPage", () => {
  test("renders page title", () => {
    renderRulesPage();
    expect(screen.getByText("Site Rules")).toBeInTheDocument();
  });

  test("renders subtitle", () => {
    renderRulesPage();
    expect(
      screen.getByText(
        "All members are expected to follow these rules. Ignorance is not an excuse.",
      ),
    ).toBeInTheDocument();
  });

  test("renders all section titles", () => {
    renderRulesPage();
    expect(screen.getByText("1. General Rules")).toBeInTheDocument();
    expect(screen.getByText("2. Uploading Rules")).toBeInTheDocument();
    expect(screen.getByText("3. Downloading Rules")).toBeInTheDocument();
    expect(screen.getByText("4. Chat Rules")).toBeInTheDocument();
    expect(screen.getByText("5. Ratio Requirements")).toBeInTheDocument();
  });

  test("renders rules as list items", () => {
    renderRulesPage();
    expect(
      screen.getByText(/Treat all members with respect/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Do not create multiple accounts/),
    ).toBeInTheDocument();
  });

  test("renders uploading rules", () => {
    renderRulesPage();
    expect(
      screen.getByText(/Only upload content that is allowed/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Torrents must be well-seeded after upload/),
    ).toBeInTheDocument();
  });

  test("renders downloading rules", () => {
    renderRulesPage();
    expect(screen.getByText(/Seed back what you download/)).toBeInTheDocument();
    expect(
      screen.getByText(/Do not manipulate your upload or download statistics/),
    ).toBeInTheDocument();
  });

  test("renders chat rules", () => {
    renderRulesPage();
    expect(
      screen.getByText(/Keep chat civil and on-topic/),
    ).toBeInTheDocument();
  });

  test("renders ratio requirements", () => {
    renderRulesPage();
    expect(
      screen.getByText(/must stay above the minimum threshold/),
    ).toBeInTheDocument();
  });

  test("renders consequences warning section", () => {
    renderRulesPage();
    expect(screen.getByText("Consequences for Violations")).toBeInTheDocument();
    expect(screen.getByText(/First offense:/)).toBeInTheDocument();
    expect(screen.getByText(/Second offense:/)).toBeInTheDocument();
    expect(screen.getByText(/Third offense:/)).toBeInTheDocument();
  });

  test("mentions permanent ban for severe violations", () => {
    renderRulesPage();
    expect(
      screen.getByText(/immediate permanent ban without prior warnings/),
    ).toBeInTheDocument();
  });
});
